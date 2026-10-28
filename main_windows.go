//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

//go:embed assets/info.png
var infoPNG []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pShowWindow         = user32.NewProc("ShowWindow")
	pSetWindowText      = user32.NewProc("SetWindowTextW")
	pGetWindowText      = user32.NewProc("GetWindowTextW")
	pSendMessage        = user32.NewProc("SendMessageW")
	pSetTimer           = user32.NewProc("SetTimer")
	pRegisterHotKey     = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	pFindWindow         = user32.NewProc("FindWindowW")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pClientToScreen     = user32.NewProc("ClientToScreen")
	pGetDC              = user32.NewProc("GetDC")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pGetSysColor        = user32.NewProc("GetSysColor")
	pSetCursorPos       = user32.NewProc("SetCursorPos")
	pGetCursorPos       = user32.NewProc("GetCursorPos")
	pSendInput          = user32.NewProc("SendInput")
	pSetForeground      = user32.NewProc("SetForegroundWindow")
	pGetForeground      = user32.NewProc("GetForegroundWindow")
	pIsWindow           = user32.NewProc("IsWindow")
	pIsIconic           = user32.NewProc("IsIconic")
	pLoadCursor         = user32.NewProc("LoadCursorW")
	pAdjustWindowRect   = user32.NewProc("AdjustWindowRectEx")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pKeybdEvent         = user32.NewProc("keybd_event")
	pMessageBeep        = user32.NewProc("MessageBeep")
	pPlaySound          = winmm.NewProc("PlaySoundW")
	pMoveWindow         = user32.NewProc("MoveWindow")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pSetForegroundGuide = user32.NewProc("SetForegroundWindow")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pBitBlt             = gdi32.NewProc("BitBlt")
	pGdiFlush           = gdi32.NewProc("GdiFlush")
	pGetStockObject     = gdi32.NewProc("GetStockObject")
	pGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
	pCreateFontIndirect = gdi32.NewProc("CreateFontIndirectW")
	pSetTextColor       = gdi32.NewProc("SetTextColor")
	pSetBkMode          = gdi32.NewProc("SetBkMode")
	pGetSysColorBrush   = user32.NewProc("GetSysColorBrush")

	pGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
	pGetUILanguage   = kernel32.NewProc("GetUserDefaultUILanguage")
	pTimeBeginPeriod = winmm.NewProc("timeBeginPeriod")
	pTimeEndPeriod   = winmm.NewProc("timeEndPeriod")
	pInitCommon      = comctl32.NewProc("InitCommonControls")
)

const (
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsTabstop     = 0x00010000
	wsBorder      = 0x00800000
	wsVScroll     = 0x00200000
	wsCaption     = 0x00C00000
	wsSysmenu     = 0x00080000
	wsMinimize    = 0x00020000
	wsOverlapped  = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	wsExTopmost   = 0x00000008
	cbsDropList   = 0x0003
	esNumber      = 0x2000
	esAutoHScroll = 0x0080
	esMultiline   = 0x0004
	esReadonly    = 0x0800
	esAutoVScroll = 0x0040
	bsBitmap      = 0x0080
	bsAutoCheck   = 0x0003
	ssLeft        = 0x0000

	wmDestroy = 0x0002
	wmSize    = 0x0005
	wmClose   = 0x0010
	wmSetFont = 0x0030
	wmCommand = 0x0111
	wmTimer   = 0x0113
	wmHotkey  = 0x0312

	cbAddString    = 0x0143
	cbGetCurSel    = 0x0147
	cbSetCurSel    = 0x014E
	cbResetContent = 0x014B
	cbnSelChange   = 1
	pbmSetPos      = 0x0402
	pbmSetRange    = 0x0406 // PBM_SETRANGE32
	bmSetImage     = 0x00F7
	emSetSel       = 0x00B1
	bmGetCheck     = 0x00F0
	bmSetCheck     = 0x00F1

	vkF6 = 0x75

	idCombo  = 101
	idButton = 102
	idStatus = 103
	idProg   = 104
	idLead   = 105
	idDiag   = 106
	idHint   = 107
	idStam   = 108
	idPunch  = 109
	idLang   = 110
	idGuide  = 111
	idSess   = 112
	idWarn   = 113
	idAutoE  = 120
	idAutoF  = 121
	idAutoW  = 122
)

// ---------- вспомогательное ----------

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func p16(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }

type point struct{ x, y int32 }
type rect struct{ left, top, right, bottom int32 }

// ---------- окружение игры (захват экрана и ввод) ----------

type bmpEntry struct {
	bmp  uintptr
	bits unsafe.Pointer
}

type winEnv struct {
	mu       sync.Mutex
	start    time.Time
	hwnd     uintptr
	screenDC uintptr
	memDC    uintptr
	bmps     map[[2]int]bmpEntry
}

func newWinEnv() *winEnv {
	dc, _, _ := pGetDC.Call(0)
	mem, _, _ := pCreateCompatibleDC.Call(dc)
	return &winEnv{start: time.Now(), screenDC: dc, memDC: mem, bmps: map[[2]int]bmpEntry{}}
}

func (w *winEnv) game() uintptr {
	if w.hwnd != 0 {
		if r, _, _ := pIsWindow.Call(w.hwnd); r != 0 {
			return w.hwnd
		}
	}
	h, _, _ := pFindWindow.Call(p16("WINDOWSCLIENT"), p16("Roblox"))
	if h == 0 {
		h, _, _ = pFindWindow.Call(0, p16("Roblox"))
	}
	w.hwnd = h
	return h
}

// clientRect - положение и размер области игры на экране.
func (w *winEnv) clientRect() (ox, oy, cw, ch int, ok bool) {
	h := w.game()
	if h != 0 {
		if ic, _, _ := pIsIconic.Call(h); ic != 0 {
			return 0, 0, 0, 0, false
		}
		var rc rect
		pGetClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
		var p point
		pClientToScreen.Call(h, uintptr(unsafe.Pointer(&p)))
		cw, ch = int(rc.right-rc.left), int(rc.bottom-rc.top)
		if cw >= 640 && ch >= 480 {
			return int(p.x), int(p.y), cw, ch, true
		}
		return 0, 0, 0, 0, false
	}
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	return 0, 0, int(sw), int(sh), sw > 0
}

func (w *winEnv) ClientSize() (int, int, bool) {
	_, _, cw, ch, ok := w.clientRect()
	return cw, ch, ok
}

type bitmapInfo struct {
	size          uint32
	width, height int32
	planes, bpp   uint16
	compression   uint32
	sizeImage     uint32
	xppm, yppm    int32
	clrUsed       uint32
	clrImportant  uint32
	colors        [1]uint32
}

func (w *winEnv) bitmap(cw, ch int) bmpEntry {
	key := [2]int{cw, ch}
	if e, ok := w.bmps[key]; ok {
		return e
	}
	bi := bitmapInfo{size: 40, width: int32(cw), height: -int32(ch), planes: 1, bpp: 32}
	var bits unsafe.Pointer
	h, _, _ := pCreateDIBSection.Call(w.screenDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	e := bmpEntry{bmp: h, bits: bits}
	w.bmps[key] = e
	return e
}

func (w *winEnv) Grab(r Rect) *Frame {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r.W <= 0 || r.H <= 0 {
		return NewFrame(1, 1)
	}
	f := NewFrame(r.W, r.H)
	ox, oy, _, _, ok := w.clientRect()
	if !ok {
		return f
	}
	e := w.bitmap(r.W, r.H)
	if e.bmp == 0 || e.bits == nil {
		return f
	}
	pSelectObject.Call(w.memDC, e.bmp)
	const srccopyCapture = 0x00CC0020 | 0x40000000
	pBitBlt.Call(w.memDC, 0, 0, uintptr(r.W), uintptr(r.H), w.screenDC, uintptr(ox+r.X), uintptr(oy+r.Y), srccopyCapture)
	pGdiFlush.Call()
	copy(f.Pix, unsafe.Slice((*byte)(e.bits), r.W*r.H*4))
	return f
}

func (w *winEnv) Now() float64 { return time.Since(w.start).Seconds() }
func (w *winEnv) Sleep(ms float64) {
	time.Sleep(time.Duration(ms * float64(time.Millisecond)))
}
func (w *winEnv) Done() bool { return false }

// GameActive - окно Roblox сейчас на переднем плане.
func (w *winEnv) GameActive() bool {
	h := w.game()
	if h == 0 {
		return true
	}
	fg, _, _ := pGetForeground.Call()
	return fg == h
}

type mouseInput struct {
	typ       uint32
	_         uint32
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	extra     uintptr
}

type keyInput struct {
	typ   uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	extra uintptr
	_     [8]byte
}

func (w *winEnv) mouseEvent(flags uint32) {
	in := mouseInput{typ: 0, flags: flags}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func (w *winEnv) keyEvent(vk, scan uint16, up bool) {
	in := keyInput{typ: 1, vk: vk, scan: scan}
	if up {
		in.flags = 0x0002
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

// курсор должен быть над окном игры, иначе клик уйдёт в другое окно
func (w *winEnv) guardCursor() {
	ox, oy, cw, ch, ok := w.clientRect()
	if !ok {
		return
	}
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if int(p.x) < ox || int(p.y) < oy || int(p.x) >= ox+cw || int(p.y) >= oy+ch {
		pSetCursorPos.Call(uintptr(ox+cw/2), uintptr(oy+ch/2))
	}
}

func (w *winEnv) MouseDown() { w.guardCursor(); w.mouseEvent(0x0002) }
func (w *winEnv) MouseUp()   { w.mouseEvent(0x0004) }
func (w *winEnv) Click() {
	w.guardCursor()
	w.mouseEvent(0x0002)
	time.AfterFunc(12*time.Millisecond, func() { w.mouseEvent(0x0004) })
}

// Use - клик для использования предмета: кнопка удерживается дольше, чтобы игра точно засчитала.
func (w *winEnv) Use() {
	w.guardCursor()
	w.mouseEvent(0x0002)
	time.Sleep(60 * time.Millisecond)
	w.mouseEvent(0x0004)
}

func (w *winEnv) RClick() {
	w.guardCursor()
	w.mouseEvent(0x0008)
	time.AfterFunc(12*time.Millisecond, func() { w.mouseEvent(0x0010) })
}

// Beep - обычный системный сигнал "Exclamation", но в 2 раза громче по амплитуде:
// звуковой файл Windows читается и усиливается (с мягким ограничением), потом играется из памяти.
func (w *winEnv) Beep() {
	if alertWav != nil {
		if r, _, _ := pPlaySound.Call(uintptr(unsafe.Pointer(&alertWav[0])), 0, 0x0004|0x0001|0x0002); r != 0 {
			return
		}
	}
	pMessageBeep.Call(0x30)
}

var alertWav = loadAlertWav()

func loadAlertWav() []byte {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	b, err := os.ReadFile(filepath.Join(root, "Media", "Windows Exclamation.wav"))
	if err != nil || len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil
	}
	pcm := false
	for p := 12; p+8 <= len(b); {
		id, sz := string(b[p:p+4]), int(uint32(b[p+4])|uint32(b[p+5])<<8|uint32(b[p+6])<<16|uint32(b[p+7])<<24)
		body := p + 8
		if id == "fmt " && body+16 <= len(b) {
			format := int(b[body]) | int(b[body+1])<<8
			bits := int(b[body+14]) | int(b[body+15])<<8
			pcm = format == 1 && bits == 16
		}
		if id == "data" {
			if !pcm {
				return nil
			}
			end := body + sz
			if end > len(b) || sz < 0 {
				end = len(b)
			}
			for i := body; i+1 < end; i += 2 {
				v := float64(int16(uint16(b[i]) | uint16(b[i+1])<<8))
				v *= 2.0 // усиление x2; выше порога - мягкое ограничение, чтобы не хрипело
				if av := math.Abs(v); av > 24000 {
					v = math.Copysign(24000+8767*math.Tanh((av-24000)/8767), v)
				}
				u := uint16(int16(v))
				b[i], b[i+1] = byte(u), byte(u>>8)
			}
			return b
		}
		p = body + sz + sz%2
	}
	return nil
}

func (w *winEnv) KeyE() {
	w.keyEvent(0x45, 0x12, false)
	time.Sleep(45 * time.Millisecond)
	w.keyEvent(0x45, 0x12, true)
}

// Key нажимает цифровую клавишу n (слот панели предметов).
func (w *winEnv) Key(n int) {
	if n < 1 || n > 9 {
		return
	}
	w.keyEvent(uint16(0x30+n), uint16(1+n), false)
	time.Sleep(45 * time.Millisecond)
	w.keyEvent(uint16(0x30+n), uint16(1+n), true)
}

func (w *winEnv) EnsureFocus() {
	h := w.game()
	if h == 0 {
		return
	}
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		pShowWindow.Call(h, 9) // SW_RESTORE
	}
	if fg, _, _ := pGetForeground.Call(); fg != h {
		pKeybdEvent.Call(0x12, 0, 0, 0) // Alt вниз/вверх - разрешает смену фокуса
		pKeybdEvent.Call(0x12, 0, 2, 0)
		pSetForeground.Call(h)
		time.Sleep(80 * time.Millisecond)
	}
	w.guardCursor()
}

// ---------- настройки ----------

type statDef struct {
	key    string // ключ названия
	hint   string // ключ подсказки
	gloves bool
	mode   int // жёстко привязанный режим: 1 клики, 2 зажатие, 3 груша
}

var statDefs = []statDef{
	{"stat.arm", "hint.bar", false, 2},
	{"stat.leg", "hint.bar", false, 2},
	{"stat.torso", "hint.bar", false, 2},
	{"stat.pspeed", "hint.pspeed", true, 1},
	{"stat.jump", "hint.bar", false, 1},
	{"stat.sharp", "hint.sharp", false, 3},
}

type saved struct {
	Stat    int  `json:"stat"`
	LeadMs  int  `json:"lead_ms"`
	PunchMs int  `json:"punch_ms"`
	Lang    int  `json:"lang"`
	LangSet bool `json:"lang_set"`
	WinX    int  `json:"win_x"`
	WinY    int  `json:"win_y"`
	HasPos  bool `json:"has_pos"`
	AutoE   bool `json:"auto_energy"`
	AutoF   bool `json:"auto_food"`
	AutoW   bool `json:"auto_water"`
}

var (
	curMode  int32
	curLead  uint64
	curPunch uint64
	curLang  int32
	settings = saved{Stat: 0, LeadMs: 35, PunchMs: 300, AutoE: true, AutoF: true, AutoW: true}
	curAuto  uint32
	dpi      = 96

	hMain, hCombo, hButton, hStatus, hProg, hLead, hHint, hStam, hPunch, hLangBox, hSess, hGuideBtn uintptr
	hWarn, hWarnText, hFontBold                                                                     uintptr
	hAutoE, hAutoF, hAutoW                                                                          uintptr
	hGuide, hGuideEdit, hFont                                                                       uintptr
	engine                                                                                          *Engine
	env                                                                                             *winEnv

	labels []labelRef

	stMu      sync.Mutex
	stKey     = "st.ready"
	stArgs    []any
	stStam    = -1.0
	lastText  = ""
	lastBtn   = ""
	lastStam  = ""
	lastSess  = ""
	lastHint  = ""
	lastWarn  = uint32(0)
	winStyle  uint32
	statNames []string
)

type labelRef struct {
	h   uintptr
	key string
}

func sc(n int) int { return n * dpi / 96 }

func lang() Lang                       { return Lang(atomic.LoadInt32(&curLang)) }
func L(key string, args ...any) string { return T(lang(), key, args...) }

func settingsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "hajime_settings.json"
	}
	return filepath.Join(filepath.Dir(exe), "hajime_settings.json")
}

func loadSettings() {
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &settings)
	}
	if settings.Stat < 0 || settings.Stat >= len(statDefs) {
		settings.Stat = 0
	}
	if settings.LeadMs < 0 || settings.LeadMs > 300 {
		settings.LeadMs = 35
	}
	if settings.PunchMs < 50 || settings.PunchMs > 3000 {
		settings.PunchMs = 300
	}
	if !settings.LangSet || settings.Lang < 0 || settings.Lang > 2 {
		settings.Lang = int(LangEN) // по умолчанию всегда английский
	}
}

func saveSettings() {
	if b, err := json.Marshal(settings); err == nil {
		os.WriteFile(settingsPath(), b, 0644)
	}
}

func applyStat(i int) {
	if i < 0 || i >= len(statDefs) {
		return
	}
	settings.Stat = i
	atomic.StoreInt32(&curMode, int32(statDefs[i].mode))
	lastHint = ""
	saveSettings()
}

func autoMask() uint32 {
	var v uint32
	if settings.AutoE {
		v |= AutoEnergy
	}
	if settings.AutoF {
		v |= AutoFood
	}
	if settings.AutoW {
		v |= AutoWater
	}
	return v
}

func applyAuto() { atomic.StoreUint32(&curAuto, autoMask()) }

func applyLead(ms int) {
	settings.LeadMs = ms
	atomic.StoreUint64(&curLead, math.Float64bits(float64(ms)))
}

func applyPunch(ms int) {
	settings.PunchMs = ms
	atomic.StoreUint64(&curPunch, math.Float64bits(float64(ms)))
}

func getText(h uintptr) string {
	buf := make([]uint16, 64)
	n, _, _ := pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), 64)
	return syscall.UTF16ToString(buf[:n])
}

func setText(h uintptr, s string) { pSetWindowText.Call(h, p16(s)) }

func fmtDur(sec float64) string {
	s := int(sec)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
}

// fillStats перезаполняет список статов (при смене языка названия меняются).
func fillStats() {
	pSendMessage.Call(hCombo, cbResetContent, 0, 0)
	for _, s := range statDefs {
		pSendMessage.Call(hCombo, cbAddString, 0, p16(L(s.key)))
	}
	pSendMessage.Call(hCombo, cbSetCurSel, uintptr(settings.Stat), 0)
}

func setLang(i int) {
	if i < 0 || i > 2 {
		return
	}
	atomic.StoreInt32(&curLang, int32(i))
	settings.Lang, settings.LangSet = i, true
	saveSettings()
	for _, l := range labels {
		setText(l.h, L(l.key))
	}
	setText(hWarn, L("ui.warning"))
	lastWarn = ^uint32(0)
	fillStats()
	lastText, lastBtn, lastStam, lastSess, lastHint = "", "", "", "", ""
	if hGuide != 0 {
		setText(hGuide, L("guide.title"))
		setText(hGuideEdit, guideText())
		pSendMessage.Call(hGuideEdit, emSetSel, 0, 0)
	}
}

func guideText() string { return strings.ReplaceAll(L("guide.text"), "\n", "\r\n") }

func refreshUI() {
	if v, err := strconv.Atoi(strings.TrimSpace(getText(hLead))); err == nil && v >= 0 && v <= 300 && v != settings.LeadMs {
		applyLead(v)
		saveSettings()
	}
	if v, err := strconv.Atoi(strings.TrimSpace(getText(hPunch))); err == nil && v >= 50 && v <= 3000 && v != settings.PunchMs {
		applyPunch(v)
		saveSettings()
	}
	stMu.Lock()
	key, args, stam := stKey, stArgs, stStam
	stMu.Unlock()
	if text := L(key, args...); text != lastText {
		lastText = text
		setText(hStatus, text)
	}
	btn := L("ui.start")
	if engine.Running() {
		btn = L("ui.stop")
	}
	if btn != lastBtn {
		lastBtn = btn
		setText(hButton, btn)
	}
	st := L("ui.stam_none")
	pos := uintptr(0)
	if stam >= 0 {
		pos = uintptr(stam*100 + 0.5)
		st = L("ui.stamina", int(stam*100+0.5))
	}
	pSendMessage.Call(hProg, pbmSetPos, pos, 0)
	if st != lastStam {
		lastStam = st
		setText(hStam, st)
	}
	if fl := engine.Warnings(); fl != lastWarn {
		lastWarn = fl
		showWarnings(fl)
	}
	tr, pu, sec := engine.SessionStats()
	if s := L("ui.session", tr, pu, fmtDur(sec)); s != lastSess {
		lastSess = s
		setText(hSess, s)
	}
	if h := L(statDefs[settings.Stat].hint); h != lastHint {
		lastHint = h
		setText(hHint, h)
	}
}

const baseClientH = 460

// showWarnings показывает список активных предупреждений в отведённой для них области окна.
func showWarnings(fl uint32) {
	var lines []string
	if fl&WarnCap != 0 {
		lines = append(lines, "- "+L("warn.cap"))
	}
	if fl&WarnEnergy != 0 {
		lines = append(lines, "- "+L("warn.energy"))
	}
	if fl&WarnGloves != 0 {
		lines = append(lines, "- "+L("warn.gloves"))
	}
	if len(lines) == 0 {
		pShowWindow.Call(hWarn, 0)
		pShowWindow.Call(hWarnText, 0)
		return
	}
	setText(hWarn, L("ui.warning"))
	setText(hWarnText, strings.Join(lines, "\r\n"))
	pShowWindow.Call(hWarn, 5)
	pShowWindow.Call(hWarnText, 5)
}

func saveDiag() {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	w, h, ok := env.ClientSize()
	var sb strings.Builder
	fmt.Fprintf(&sb, "game window: ok=%v %dx%d dpi=%d\n", ok, w, h, dpi)
	if !ok {
		os.WriteFile(filepath.Join(dir, "hajime_diag.txt"), []byte(sb.String()), 0644)
		return
	}
	f := env.Grab(Rect{0, 0, w, h})
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = f.Pix[i*4+2], f.Pix[i*4+1], f.Pix[i*4], 255
	}
	if fh, err := os.Create(filepath.Join(dir, "hajime_diag.png")); err == nil {
		png.Encode(fh, img)
		fh.Close()
	}
	bar, found := FindBar(f)
	fmt.Fprintf(&sb, "timing bar found: %v %+v\n", found, bar)
	if found {
		sr := StripRect(bar)
		st := cropFrame(f, sr)
		lb := LocalBar(bar)
		x, iok := FindIndicator(st, lb)
		fmt.Fprintf(&sb, "indicator: %v x=%.1f (bar center %.1f)\n", iok, x+float64(sr.X), bar.Center())
		l, r, bok := FindBox(st, lb)
		fmt.Fprintf(&sb, "white bar: %v %d..%d\n", bok, l+sr.X, r+sr.X)
	}
	g, sok := LocateStamina(f)
	fmt.Fprintf(&sb, "stamina bar located: %v %+v scale=%.2f\n", sok, g, g.Scale())
	if sok {
		fr, rok := Stamina(cropFrame(f, StaminaRect(g)))
		fmt.Fprintf(&sb, "stamina: %v %.3f\n", rok, fr)
		gl := Gloves(cropFrame(f, HotbarRect(g, w, h)), g.Scale())
		fmt.Fprintf(&sb, "gloves (0 hotbar not visible, 1 equipped, 2 not equipped): %d\n", gl)
	}
	en, fo, wa := engine.LastSupplies()
	fmt.Fprintf(&sb, "supplies (last read, -1 = none): energy %.2f food %.2f water %.2f\n", en, fo, wa)
	if f2 := env.Grab(Rect{0, 0, w, h}); f2 != nil {
		sp := ReadSupplies(f2)
		fmt.Fprintf(&sb, "supplies now: ok=%v energy %.2f food %.2f water %.2f\n", sp.OK, sp.Energy, sp.Food, sp.Water)
	}
	fmt.Fprintf(&sb, "mode=%d lead=%dms punch delay=%dms lang=%d\n", atomic.LoadInt32(&curMode), settings.LeadMs, settings.PunchMs, settings.Lang)
	os.WriteFile(filepath.Join(dir, "hajime_diag.txt"), []byte(sb.String()), 0644)
	setStatus("st.diag_saved", nil, -2)
}

func setStatus(key string, args []any, stam float64) {
	stMu.Lock()
	stKey, stArgs = key, args
	if stam > -1.5 {
		stStam = stam
	}
	stMu.Unlock()
}

// ---------- иконка кнопки руководства ----------

// makeIconBitmap уменьшает встроенную картинку до size px и накладывает её на цвет кнопки.
func makeIconBitmap(size int) uintptr {
	img, err := png.Decode(bytes.NewReader(infoPNG))
	if err != nil {
		return 0
	}
	c, _, _ := pGetSysColor.Call(15) // COLOR_BTNFACE
	fr, fg, fb := float64(c&0xFF), float64((c>>8)&0xFF), float64((c>>16)&0xFF)
	bi := bitmapInfo{size: 40, width: int32(size), height: -int32(size), planes: 1, bpp: 32}
	var bits unsafe.Pointer
	hb, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hb == 0 || bits == nil {
		return 0
	}
	dst := unsafe.Slice((*byte)(bits), size*size*4)
	sb := img.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for y := 0; y < size; y++ {
		y0, y1 := y*sh/size, (y+1)*sh/size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0, x1 := x*sw/size, (x+1)*sw/size
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sr, sg, sbl, sa, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, b, a := img.At(sb.Min.X+xx, sb.Min.Y+yy).RGBA() // с предумножением
					sr, sg, sbl, sa, n = sr+float64(r), sg+float64(g), sbl+float64(b), sa+float64(a), n+1
				}
			}
			sr, sg, sbl, sa = sr/n/257, sg/n/257, sbl/n/257, sa/n/257 // 0..255
			inv := 1 - sa/255
			i := (y*size + x) * 4
			dst[i] = byte(sbl + fb*inv)
			dst[i+1] = byte(sg + fg*inv)
			dst[i+2] = byte(sr + fr*inv)
			dst[i+3] = 255
		}
	}
	return hb
}

// ---------- окна ----------

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmCommand:
		id, code := wp&0xFFFF, (wp>>16)&0xFFFF
		switch {
		case id == idButton && code == 0:
			engine.Send(CmdToggle)
		case id == idDiag && code == 0:
			go saveDiag()
		case id == idGuide && code == 0:
			openGuide()
		case (id == idAutoE || id == idAutoF || id == idAutoW) && code == 0:
			on := func(h uintptr) bool { r, _, _ := pSendMessage.Call(h, bmGetCheck, 0, 0); return r == 1 }
			switch id {
			case idAutoE:
				settings.AutoE = on(hAutoE)
			case idAutoF:
				settings.AutoF = on(hAutoF)
			case idAutoW:
				settings.AutoW = on(hAutoW)
			}
			applyAuto()
			saveSettings()
		case id == idCombo && code == cbnSelChange:
			i, _, _ := pSendMessage.Call(hCombo, cbGetCurSel, 0, 0)
			applyStat(int(i))
		case id == idLang && code == cbnSelChange:
			i, _, _ := pSendMessage.Call(hLangBox, cbGetCurSel, 0, 0)
			setLang(int(i))
		}
		return 0
	case 0x0138: // WM_CTLCOLORSTATIC
		if (lp == hWarn && hWarn != 0) || (lp == hWarnText && hWarnText != 0) {
			pSetTextColor.Call(wp, 0x0000C8) // красный (BGR)
			pSetBkMode.Call(wp, 1)
			b, _, _ := pGetSysColorBrush.Call(15)
			return b
		}
	case wmHotkey:
		engine.Send(CmdToggle)
		return 0
	case wmTimer:
		refreshUI()
		return 0
	case wmClose:
		var rc rect
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		settings.WinX, settings.WinY, settings.HasPos = int(rc.left), int(rc.top), true
		saveSettings()
	case wmDestroy:
		engine.Send(CmdQuit)
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func guideProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmSize:
		w, h := lp&0xFFFF, (lp>>16)&0xFFFF
		pMoveWindow.Call(hGuideEdit, 0, 0, w, h, 1)
		return 0
	case wmDestroy:
		hGuide, hGuideEdit = 0, 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

func registerClass(name string, proc uintptr) {
	hi, _, _ := pGetModuleHandle.Call(0)
	cur, _, _ := pLoadCursor.Call(0, 32512)
	cls := u16(name)
	wc := wndClassEx{instance: hi, cursor: cur, background: 16, className: cls, wndProc: proc}
	wc.size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
}

func setFont(h uintptr) {
	if hFont != 0 {
		pSendMessage.Call(h, wmSetFont, hFont, 1)
	}
}

// child создаёт дочерний элемент; размеры задаются для 96 dpi и масштабируются.
func child(class, text string, style uint32, exStyle uint32, x, y, w, h int, id uintptr) uintptr {
	hi, _, _ := pGetModuleHandle.Call(0)
	h2, _, _ := pCreateWindowEx.Call(uintptr(exStyle), p16(class), p16(text),
		uintptr(style|wsChild|wsVisible), uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(h)), hMain, id, hi, 0)
	setFont(h2)
	return h2
}

// label - подпись, которая переводится при смене языка.
func label(key string, x, y, w, h int) uintptr {
	hh := child("STATIC", L(key), ssLeft, 0, x, y, w, h, 0)
	labels = append(labels, labelRef{hh, key})
	return hh
}

type logFont struct {
	height, width, escapement, orientation, weight int32
	italic, underline, strikeOut, charSet          byte
	outPrecision, clipPrecision, quality, pitch    byte
	face                                           [32]uint16
}

func makeBoldFont() {
	lf := logFont{height: -int32(9 * dpi / 72), weight: 700, charSet: 1, quality: 5}
	copy(lf.face[:], syscall.StringToUTF16("Segoe UI"))
	hFontBold, _, _ = pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf)))
}

func makeFont() {
	lf := logFont{height: -int32(9 * dpi / 72), weight: 400, charSet: 1, quality: 5}
	copy(lf.face[:], syscall.StringToUTF16("Segoe UI"))
	hFont, _, _ = pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf)))
	if hFont == 0 {
		hFont, _, _ = pGetStockObject.Call(17)
	}
}

func openGuide() {
	if hGuide != 0 {
		pShowWindow.Call(hGuide, 9)
		pSetForegroundGuide.Call(hGuide)
		return
	}
	hi, _, _ := pGetModuleHandle.Call(0)
	var rc rect
	pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	hGuide, _, _ = pCreateWindowEx.Call(0, p16("HajimeGuideWnd"), p16(L("guide.title")), wsOverlapped|wsVisible,
		uintptr(rc.right)+uintptr(sc(8)), uintptr(rc.top), uintptr(sc(520)), uintptr(sc(560)), 0, 0, hi, 0)
	hGuideEdit, _, _ = pCreateWindowEx.Call(0, p16("EDIT"), p16(guideText()),
		wsChild|wsVisible|wsVScroll|esMultiline|esReadonly|esAutoVScroll, 0, 0, uintptr(sc(500)), uintptr(sc(520)), hGuide, 0, hi, 0)
	setFont(hGuideEdit)
	pSendMessage.Call(hGuideEdit, emSetSel, 0, 0)
	pShowWindow.Call(hGuide, 5)
}

func main() {
	runtime.LockOSThread()
	pSetProcessDPIAware.Call()
	pTimeBeginPeriod.Call(1)
	defer pTimeEndPeriod.Call(1)
	pInitCommon.Call()

	loadSettings()
	atomic.StoreInt32(&curLang, int32(settings.Lang))
	atomic.StoreInt32(&curMode, int32(statDefs[settings.Stat].mode))
	atomic.StoreUint64(&curLead, math.Float64bits(float64(settings.LeadMs)))
	atomic.StoreUint64(&curPunch, math.Float64bits(float64(settings.PunchMs)))
	applyAuto()

	env = newWinEnv()
	if d, _, _ := pGetDeviceCaps.Call(env.screenDC, 88); d >= 96 { // LOGPIXELSX
		dpi = int(d)
	}
	makeFont()
	makeBoldFont()
	engine = NewEngine(env, func() Settings {
		return Settings{
			Mode:    int(atomic.LoadInt32(&curMode)),
			LeadMs:  math.Float64frombits(atomic.LoadUint64(&curLead)),
			PunchMs: math.Float64frombits(atomic.LoadUint64(&curPunch)),
			Gloves:  statDefs[settings.Stat].gloves || statDefs[settings.Stat].mode == 3,
			Auto:    atomic.LoadUint32(&curAuto),
		}
	}, setStatus)
	engine.Async = true

	registerClass("HajimeMacroWnd", syscall.NewCallback(wndProc))
	registerClass("HajimeGuideWnd", syscall.NewCallback(guideProc))

	const cw, chh = 330, baseClientH
	style := uint32(wsCaption | wsSysmenu | wsMinimize)
	winStyle = style
	rc := rect{0, 0, int32(sc(cw)), int32(sc(chh))}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), uintptr(style), 0, wsExTopmost)
	wx, wy := 40, 140
	if settings.HasPos {
		vx, _, _ := pGetSystemMetrics.Call(76)
		vy, _, _ := pGetSystemMetrics.Call(77)
		vw, _, _ := pGetSystemMetrics.Call(78)
		vh, _, _ := pGetSystemMetrics.Call(79)
		if int32(settings.WinX) > int32(vx)-100 && int32(settings.WinX) < int32(vx)+int32(vw)-100 &&
			int32(settings.WinY) > int32(vy)-10 && int32(settings.WinY) < int32(vy)+int32(vh)-60 {
			wx, wy = settings.WinX, settings.WinY
		}
	}
	hi, _, _ := pGetModuleHandle.Call(0)
	hMain, _, _ = pCreateWindowEx.Call(wsExTopmost, p16("HajimeMacroWnd"), p16("Hajime Macro"),
		uintptr(style), uintptr(wx), uintptr(wy), uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top), 0, 0, hi, 0)

	child("STATIC", "Language", ssLeft, 0, 10, 10, 138, 18, 0)
	hLangBox = child("COMBOBOX", "", cbsDropList|wsVScroll|wsTabstop, 0, 150, 6, 126, 120, idLang)
	for _, n := range langNames {
		pSendMessage.Call(hLangBox, cbAddString, 0, p16(n))
	}
	pSendMessage.Call(hLangBox, cbSetCurSel, uintptr(settings.Lang), 0)
	hGuideBtn = child("BUTTON", "", bsBitmap|wsTabstop, 0, 284, 4, 36, 26, idGuide)
	if bmp := makeIconBitmap(sc(20)); bmp != 0 {
		pSendMessage.Call(hGuideBtn, bmSetImage, 0, bmp)
	} else {
		setText(hGuideBtn, "?")
	}

	hCombo = child("COMBOBOX", "", cbsDropList|wsVScroll|wsTabstop, 0, 10, 36, 310, 200, idCombo)
	fillStats()
	hHint = child("STATIC", "", ssLeft, 0, 10, 68, 310, 34, idHint)
	hButton = child("BUTTON", L("ui.start"), wsTabstop, 0, 10, 106, 310, 40, idButton)
	mk := func(key string, x, id int, on bool) uintptr {
		h := child("BUTTON", L(key), bsAutoCheck|wsTabstop, 0, x, 150, 104, 22, uintptr(id))
		labels = append(labels, labelRef{h, key})
		v := uintptr(0)
		if on {
			v = 1
		}
		pSendMessage.Call(h, bmSetCheck, v, 0)
		return h
	}
	hAutoE = mk("ui.auto_e", 10, idAutoE, settings.AutoE)
	hAutoF = mk("ui.auto_f", 114, idAutoF, settings.AutoF)
	hAutoW = mk("ui.auto_w", 218, idAutoW, settings.AutoW)
	hStatus = child("STATIC", "", ssLeft, 0, 10, 182, 310, 38, idStatus)
	hWarn = child("STATIC", L("ui.warning"), ssLeft, 0, 10, 222, 310, 18, idWarn)
	if hFontBold != 0 {
		pSendMessage.Call(hWarn, wmSetFont, hFontBold, 1)
	}
	hWarnText = child("STATIC", "", ssLeft, 0, 10, 240, 310, 52, idWarn+1)
	pShowWindow.Call(hWarn, 0)
	pShowWindow.Call(hWarnText, 0)
	hStam = child("STATIC", "", ssLeft, 0, 10, 296, 310, 16, idStam)
	hProg = child("msctls_progress32", "", 0, 0, 10, 314, 310, 14, idProg)
	pSendMessage.Call(hProg, pbmSetRange, 0, 100)
	hSess = child("STATIC", "", ssLeft, 0, 10, 336, 310, 16, idSess)
	label("ui.lead", 10, 364, 120, 18)
	hLead = child("EDIT", strconv.Itoa(settings.LeadMs), esNumber|esAutoHScroll|wsTabstop|wsBorder, 0, 134, 361, 48, 22, idLead)
	label("ui.lead_hint", 190, 364, 130, 18)
	label("ui.punch", 10, 392, 150, 18)
	hPunch = child("EDIT", strconv.Itoa(settings.PunchMs), esNumber|esAutoHScroll|wsTabstop|wsBorder, 0, 164, 389, 52, 22, idPunch)
	label("ui.punch_hint", 224, 392, 96, 18)
	diagBtn := child("BUTTON", L("ui.diag"), wsTabstop, 0, 10, 424, 120, 26, idDiag)
	labels = append(labels, labelRef{diagBtn, "ui.diag"})
	label("ui.hotkey", 140, 429, 180, 18)

	pShowWindow.Call(hMain, 5)
	pSetTimer.Call(hMain, 1, 200, 0)
	refreshUI()
	if r, _, _ := pRegisterHotKey.Call(hMain, 1, 0x4000, vkF6); r == 0 {
		setStatus("st.hotkey_busy", nil, -1)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				setStatus("st.error", []any{r}, -1)
			}
		}()
		engine.Run()
	}()

	var m struct {
		hwnd    uintptr
		message uint32
		_       uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      point
		_       uint32
	}
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	pUnregisterHotKey.Call(hMain, 1)
}
