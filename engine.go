package main

import (
	"math"
	"sync/atomic"
)

// Env - всё, что нужно движку от окружения (на Windows настоящее, в тестах - симуляция).
type Env interface {
	ClientSize() (w, h int, ok bool) // размер клиентской области окна игры
	Grab(r Rect) *Frame              // кадр области (координаты клиентской области игры)
	Now() float64                    // секунды, монотонные
	Sleep(ms float64)
	MouseDown()
	MouseUp()
	Click()
	RClick()
	Beep()            // звуковое предупреждение
	KeyE()            // нажать и отпустить E
	EnsureFocus()     // вынести игру на передний план и убрать курсор в её окно
	GameActive() bool // окно игры сейчас активно (на переднем плане)
	Done() bool       // true - пора выходить (для тестов)
}

// fullWait - пауза после полной стамины перед нажатием E, секунд.
const fullWait = 1.0

// bagEmpty - при такой стамине и ниже считаем, что бить нечем.
const bagEmpty = 0.03

type Cmd int

const (
	CmdToggle Cmd = iota // F6 / кнопка Старт-Стоп
	CmdQuit
)

// Режимы: 0 - полоска, тип определяется сама; 1 и 2 - принудительно;
// 3 - груша (резкость удара), полоски нет.
type Settings struct {
	Mode    int
	LeadMs  float64 // опережение: компенсация задержек захвата экрана и ввода
	PunchMs float64 // режим 3: пауза между ударами
	Gloves  bool    // для полоски: нужны ли перчатки (Punch speed)
}

type state int

const (
	stIdle state = iota
	stSeek
	stWaitBar
	stTrain
	stBag
)

type sample struct{ t, x float64 }

// StatusFn получает ключ сообщения (см. i18n.go), его аргументы и стамину (-1 - неизвестна).
type StatusFn func(key string, args []any, stamina float64)

type Engine struct {
	env    Env
	cmds   chan Cmd
	set    func() Settings
	status StatusFn

	st        state
	runFlag   int32
	fullSince float64
	retries   int
	deadline  float64
	paused    bool

	// геометрия интерфейса
	stam     StamGeom
	stamOK   bool
	stamSize [2]int

	bar        Bar
	strip      Rect
	lbar       Bar
	curMode    int  // 1 или 2 - фактический тип полоски
	needRefine bool // полоска найдена, когда индикатор закрывал её край - уточнить позже
	lastSeen   float64
	lastStam   float64
	lastStat   float64

	ring []sample

	// режим 1
	armed        bool
	approachSide int
	prevD        float64
	hasPrevD     bool
	lastFire     float64

	// режим 2
	boxKnown, boxFrozen bool
	boxL, boxR          float64
	seenLeft            bool
	pressed, holding    bool
	pressT              float64

	// режим 3 (груша)
	bagRest    bool
	bagLeft    bool
	nextPunch  float64
	glvWarned  bool
	bagCheckAt float64
	bagFr      float64
	bagSok     bool
	bagGl      GlovesState

	// предупреждения (WarnGloves/WarnCap/WarnEnergy), читаются из интерфейса
	warn       uint32
	capSeen    bool
	capChecks  int
	capRecheck float64
	warnNext   float64
	bagEnergy  bool

	// статистика сессии
	trainings int64
	punches   int64
	runAccum  int64 // нс
	runSince  int64 // нс, 0 - не идёт
}

func NewEngine(env Env, set func() Settings, status StatusFn) *Engine {
	return &Engine{env: env, cmds: make(chan Cmd, 8), set: set, status: status, lastStam: -1, fullSince: -1}
}

func (e *Engine) Send(c Cmd) {
	select {
	case e.cmds <- c:
	default:
	}
}

func (e *Engine) setState(s state) {
	was := e.st
	e.st = s
	now := int64(e.env.Now() * 1e9)
	if s == stIdle {
		atomic.StoreInt32(&e.runFlag, 0)
		if was != stIdle {
			if since := atomic.LoadInt64(&e.runSince); since != 0 {
				atomic.AddInt64(&e.runAccum, now-since)
				atomic.StoreInt64(&e.runSince, 0)
			}
		}
	} else {
		atomic.StoreInt32(&e.runFlag, 1)
		if was == stIdle {
			atomic.StoreInt64(&e.runSince, now+1) // +1: ноль зарезервирован под "не идёт"
		}
	}
}

// Running - true, пока макрос запущен (для надписи на кнопке).
func (e *Engine) Running() bool { return atomic.LoadInt32(&e.runFlag) == 1 }

// SessionStats - тренировок, ударов и время работы (секунды) за сессию.
func (e *Engine) SessionStats() (trainings, punches int64, seconds float64) {
	acc := atomic.LoadInt64(&e.runAccum)
	if since := atomic.LoadInt64(&e.runSince); since != 0 {
		acc += int64(e.env.Now()*1e9) - since
	}
	return atomic.LoadInt64(&e.trainings), atomic.LoadInt64(&e.punches), float64(acc) / 1e9
}

func (e *Engine) say(key string, args ...any) { e.status(key, args, e.lastStam) }

// Предупреждения: можно показывать несколько сразу.
const (
	WarnGloves uint32 = 1 << iota // перчатки не надеты
	WarnCap                       // кап стата достигнут (тренировка больше его не повышает)
	WarnEnergy                    // энергии меньше 10%
)

// Warnings - набор активных предупреждений (битовая маска Warn*).
func (e *Engine) Warnings() uint32 { return atomic.LoadUint32(&e.warn) }

// setWarn включает/выключает предупреждение; true, если оно только что появилось.
func (e *Engine) setWarn(bit uint32, on bool) bool {
	for {
		old := atomic.LoadUint32(&e.warn)
		nw := old &^ bit
		if on {
			nw = old | bit
		}
		if atomic.CompareAndSwapUint32(&e.warn, old, nw) {
			return on && old&bit == 0
		}
	}
}

func (e *Engine) clearWarn(bits uint32) {
	for {
		old := atomic.LoadUint32(&e.warn)
		if atomic.CompareAndSwapUint32(&e.warn, old, old&^bits) {
			return
		}
	}
}

// capScales - масштабы, в которых искать сообщение о капе: как у интерфейса, по высоте кадра и без масштаба.
func (e *Engine) capScales(h int) []float64 {
	var ks []float64
	add := func(k float64) {
		for _, v := range ks {
			if math.Abs(v-k) < 0.03 {
				return
			}
		}
		ks = append(ks, k)
	}
	if e.stamOK {
		add(e.stam.Scale())
	}
	add(float64(h) / 1078)
	add(1.0)
	return ks
}

// checkWarnings ищет на экране сообщение о капе и (если energy) читает шкалу энергии.
// При появлении нового предупреждения подаёт звуковой сигнал.
func (e *Engine) checkWarnings(energy bool) {
	w, h, ok := e.env.ClientSize()
	if !ok {
		return
	}
	raised := false
	if !e.capSeen {
		if CapMessage(e.env.Grab(CapBand(w, h)), e.capScales(h)) {
			e.capSeen = true
			raised = e.setWarn(WarnCap, true) || raised
		}
	}
	if energy {
		if low, found := EnergyLow(e.env.Grab(Rect{0, 0, w, h})); found {
			raised = e.setWarn(WarnEnergy, low) || raised
		}
	}
	if raised {
		e.env.Beep()
	}
}

// newTrainingWarnings - начало новой тренировки: предупреждения проверяются заново.
func (e *Engine) newTrainingWarnings(now float64) {
	e.clearWarn(WarnCap | WarnEnergy | WarnGloves)
	e.capSeen = false
	e.capChecks, e.capRecheck = 2, now+1.0
	e.checkWarnings(true)
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func (e *Engine) Run() {
	e.setState(stIdle)
	e.say("st.ready")
	for !e.env.Done() {
		select {
		case c := <-e.cmds:
			if c == CmdQuit {
				e.releaseAll()
				return
			}
			e.toggle()
		default:
		}
		// игра не в фокусе - пауза, чтобы не кликать по чужим окнам
		if e.st != stIdle {
			if !e.env.GameActive() {
				if !e.paused {
					e.paused = true
					e.releaseAll()
				}
				e.say("st.paused")
				e.env.Sleep(100)
				continue
			}
			if e.paused {
				e.paused = false
				e.onResume()
			}
		} else {
			e.paused = false
		}
		switch e.st {
		case stIdle:
			e.idleTick()
		case stSeek:
			e.seekTick()
		case stWaitBar:
			e.waitTick()
		case stTrain:
			e.trainTick()
		case stBag:
			e.bagTick()
		}
	}
	e.releaseAll()
}

func (e *Engine) onResume() {
	now := e.env.Now()
	switch e.st {
	case stTrain:
		e.resetRound()
		e.hasPrevD, e.armed = false, true
		e.lastSeen = now
	case stWaitBar:
		e.deadline = now + 3.5
	case stBag:
		e.nextPunch = now + 0.3
	}
}

func (e *Engine) releaseAll() {
	if e.holding {
		e.env.MouseUp()
		e.holding = false
	}
}

func (e *Engine) toggle() {
	if e.st == stIdle {
		e.retries = 0
		e.fullSince = -1
		e.stamOK = false
		e.env.EnsureFocus()
		if e.set().Mode == 3 {
			e.bagRest, e.bagLeft, e.glvWarned = false, true, false
			e.bagCheckAt = 0
			e.clearWarn(WarnCap | WarnEnergy | WarnGloves)
			e.capSeen, e.bagEnergy, e.warnNext = false, true, e.env.Now()
			e.nextPunch = e.env.Now()
			e.setState(stBag)
			e.say("st.bag")
			return
		}
		e.setState(stSeek)
		e.say("st.starting")
		return
	}
	// остановка: если тренировка идёт - нажимаем E, чтобы её прервать
	e.releaseAll()
	if w, h, ok := e.env.ClientSize(); ok && e.st != stBag {
		if _, found := FindBar(e.env.Grab(Rect{0, 0, w, h})); found {
			e.env.EnsureFocus()
			e.env.KeyE()
		}
	}
	atomic.StoreUint32(&e.warn, 0)
	e.setState(stIdle)
	e.say("st.stopped")
}

// locateStamina находит шкалу стамины (и заодно масштаб интерфейса) в полном кадре.
func (e *Engine) locateStamina(full *Frame) bool {
	g, ok := LocateStamina(full)
	e.stamOK = ok
	if ok {
		e.stam = g
		e.stamSize = [2]int{full.W, full.H}
	}
	return ok
}

// readStamina читает стамину; при необходимости сначала ищет шкалу на экране.
func (e *Engine) readStamina() (float64, bool) {
	w, h, ok := e.env.ClientSize()
	if !ok {
		return 0, false
	}
	if !e.stamOK || e.stamSize != [2]int{w, h} {
		if !e.locateStamina(e.env.Grab(Rect{0, 0, w, h})) {
			return 0, false
		}
	}
	fr, ok := Stamina(e.env.Grab(StaminaRect(e.stam)))
	if !ok {
		e.stamOK = false
		return 0, false
	}
	e.lastStam = fr
	return fr, true
}

func (e *Engine) idleTick() {
	now := e.env.Now()
	if now-e.lastStat > 0.5 {
		e.lastStat = now
		if _, ok := e.readStamina(); !ok {
			e.lastStam = -1
		}
		e.say("st.idle")
	}
	e.env.Sleep(20)
}

func (e *Engine) seekTick() {
	w, h, ok := e.env.ClientSize()
	if !ok {
		e.say("st.nowin")
		e.env.Sleep(300)
		return
	}
	full := e.env.Grab(Rect{0, 0, w, h})
	if bar, found := FindBar(full); found {
		e.beginTrain(bar)
		return
	}
	if !e.locateStamina(full) {
		e.lastStam = -1
		e.say("st.nostam")
		e.env.Sleep(300)
		return
	}
	fr, sok := Stamina(cropFrame(full, StaminaRect(e.stam)))
	if !sok {
		e.lastStam = -1
		e.say("st.nostam")
		e.env.Sleep(300)
		return
	}
	e.lastStam = fr
	now := e.env.Now()
	if e.set().Gloves && e.glovesNow() == GlovesOff {
		if !e.glvWarned {
			e.glvWarned = true
			e.env.Beep()
		}
		e.fullSince = -1
		e.setWarn(WarnGloves, true)
		e.say("st.nogloves")
		e.env.Sleep(150)
		return
	}
	e.setWarn(WarnGloves, false)
	e.glvWarned = false
	if fr >= 0.995 {
		// стамина полная: ждём ещё 1 секунду, потом жмём E
		if e.fullSince < 0 {
			e.fullSince = now
		}
		if now-e.fullSince >= fullWait {
			e.fullSince = -1
			e.env.EnsureFocus()
			e.env.KeyE()
			e.setState(stWaitBar)
			e.deadline = e.env.Now() + 3.5
			e.say("st.pressed_e")
			return
		}
		e.say("st.full_wait")
		e.env.Sleep(30)
		return
	}
	e.fullSince = -1
	e.say("st.wait_stam", int(fr*100))
	if fr > 0.9 {
		e.env.Sleep(60)
	} else {
		e.env.Sleep(200)
	}
}

func (e *Engine) waitTick() {
	w, h, ok := e.env.ClientSize()
	if ok {
		if bar, found := FindBar(e.env.Grab(Rect{0, 0, w, h})); found {
			e.beginTrain(bar)
			return
		}
	}
	if e.env.Now() > e.deadline {
		e.retries++
		if e.retries >= 3 {
			e.setState(stIdle)
			e.say("st.cant_start")
			return
		}
		e.setState(stSeek)
		e.env.Sleep(800)
		return
	}
	e.env.Sleep(120)
}

func cropFrame(f *Frame, r Rect) *Frame {
	o := NewFrame(r.W, r.H)
	for y := 0; y < r.H; y++ {
		sy := r.Y + y
		if sy < 0 || sy >= f.H {
			continue
		}
		x0, x1 := r.X, r.X+r.W
		dx := 0
		if x0 < 0 {
			dx = -x0
			x0 = 0
		}
		if x1 > f.W {
			x1 = f.W
		}
		if x1 > x0 {
			copy(o.Pix[(y*r.W+dx)*4:], f.Pix[(sy*f.W+x0)*4:(sy*f.W+x1)*4])
		}
	}
	return o
}

func (e *Engine) setBar(bar Bar) {
	e.bar = bar
	e.strip = StripRect(bar)
	e.lbar = LocalBar(bar)
}

// detectMode определяет тип полоски: есть светлый бар - зажатие (2), нет - клики (1).
func (e *Engine) detectMode() int {
	if m := e.set().Mode; m == 1 || m == 2 {
		return m
	}
	hits := 0
	for i := 0; i < 5; i++ {
		fr := e.env.Grab(e.strip)
		if _, _, ok := FindBox(fr, e.lbar); ok {
			hits++
		}
		e.env.Sleep(25)
	}
	if hits >= 2 {
		return 2
	}
	return 1
}

func (e *Engine) beginTrain(bar Bar) {
	e.setBar(bar)
	// если индикатор стоял у края, он мог закрыть конец заливки и сдвинуть центр
	if x, ok := FindIndicator(e.env.Grab(e.strip), e.lbar); ok {
		pos, w := x+float64(e.strip.X), float64(bar.W())
		e.needRefine = pos < float64(bar.X0)+0.12*w || pos > float64(bar.X1)-0.12*w
	} else {
		e.needRefine = true
	}
	e.curMode = e.detectMode()
	e.setState(stTrain)
	e.ring = e.ring[:0]
	e.armed, e.hasPrevD = true, false
	e.lastFire = -10
	e.resetRound()
	e.lastSeen = e.env.Now()
	e.lastStat = 0
	atomic.AddInt64(&e.trainings, 1)
	e.newTrainingWarnings(e.env.Now())
	e.say("st.training", e.curMode)
}

func (e *Engine) resetRound() {
	e.boxKnown, e.boxFrozen, e.seenLeft = false, false, false
	e.pressed = false
	e.ring = e.ring[:0]
}

func (e *Engine) trainingEnded(key string) {
	e.releaseAll()
	e.setState(stSeek)
	e.say(key)
}

// скорость индикатора по последним кадрам, px/с
func (e *Engine) velocity(t, x float64) (float64, bool) {
	for _, s := range e.ring {
		dt := t - s.t
		if dt >= 0.012 && dt <= 0.08 {
			return (x - s.x) / dt, true
		}
	}
	return 0, false
}

func (e *Engine) pushSample(t, x float64) {
	e.ring = append(e.ring, sample{t, x})
	for len(e.ring) > 0 && t-e.ring[0].t > 0.1 {
		e.ring = e.ring[1:]
	}
}

func (e *Engine) trainTick() {
	if !e.capSeen && e.capChecks > 0 && e.env.Now() >= e.capRecheck {
		// сообщение о капе может появиться не сразу - перепроверяем пару раз
		e.capChecks--
		e.capRecheck = e.env.Now() + 1.5
		e.checkWarnings(false)
	}
	fr := e.env.Grab(e.strip)
	t := e.env.Now()
	x, ok := FindIndicator(fr, e.lbar)
	if !ok {
		if e.holding && t-e.lastSeen > 0.35 {
			e.env.MouseUp()
			e.holding = false
		}
		if t-e.lastSeen > 1.2 {
			w, h, cok := e.env.ClientSize()
			if cok {
				bar, found := FindBar(e.env.Grab(Rect{0, 0, w, h}))
				if !found {
					e.trainingEnded("st.ended")
					return
				}
				if bar != e.bar { // окно изменили - переезжаем на новое положение
					e.setBar(bar)
					e.resetRound()
				}
			}
			e.lastSeen = t
		}
		e.env.Sleep(2)
		return
	}
	e.lastSeen = t
	xc := x + float64(e.strip.X)
	set := e.set()
	if e.curMode == 2 {
		e.mode2(fr, t, xc, set)
	} else {
		if e.needRefine {
			e.refineBar(xc)
		}
		e.mode1(t, xc, set)
	}
	if t-e.lastStat > 0.6 {
		e.lastStat = t
		e.readStamina()
		e.say("st.training", e.curMode)
	}
	e.env.Sleep(1)
}

func (e *Engine) mode1(t, xc float64, set Settings) {
	c := e.bar.Center()
	d := xc - c
	v, vok := e.velocity(t, xc)
	e.pushSample(t, xc)
	lead := set.LeadMs / 1000
	if !e.armed {
		// взводим снова, когда индикатор ушёл за центр на 30 px (масштаб полоски)
		if sign(d) == -e.approachSide && math.Abs(d) >= float64(e.bar.W())*0.06 {
			e.armed = true
		}
	}
	fire := false
	if e.armed && t-e.lastFire >= 0.12 {
		if e.hasPrevD && e.prevD*d < 0 {
			fire = true // пересёк центр между кадрами
		} else if vok && v != 0 && d*v < 0 {
			if tau := -d / v; tau <= lead {
				fire = true // долетит до центра за время задержки - жмём заранее
			}
		}
	}
	if fire {
		e.env.Click()
		e.lastFire = t
		e.armed = false
		if vok && v != 0 {
			e.approachSide = -sign(v)
		} else {
			e.approachSide = sign(e.prevD)
		}
	}
	e.prevD, e.hasPrevD = d, true
}

func (e *Engine) mode2(fr *Frame, t, xc float64, set Settings) {
	prev := math.NaN()
	if n := len(e.ring); n > 0 {
		prev = e.ring[n-1].x
	}
	wrap := float64(e.bar.W()) * 0.25
	if !math.IsNaN(prev) && prev-xc > wrap { // индикатор вернулся к левому краю - новый круг
		e.releaseAll()
		e.resetRound()
	}
	v, vok := e.velocity(t, xc)
	e.pushSample(t, xc)
	lead := set.LeadMs / 1000
	pre := float64(e.bar.W()) * 0.06

	if !e.boxFrozen && !e.holding && !e.pressed {
		if l, r, ok := FindBox(fr, e.lbar); ok {
			e.boxL = float64(l + e.strip.X)
			e.boxR = float64(r + e.strip.X)
			e.boxKnown = true
		}
		if e.boxKnown && xc >= e.boxL-pre {
			e.boxFrozen = true
		}
	}
	if e.boxKnown && xc < e.boxL-pre/5 {
		e.seenLeft = true
	}
	if !e.holding && !e.pressed && e.boxKnown && e.seenLeft && vok && v > 0 &&
		xc+v*lead >= e.boxL && xc < e.boxR-pre/3 {
		e.env.MouseDown()
		e.holding, e.pressed, e.pressT = true, true, t
	}
	if e.holding && ((vok && xc+v*lead >= e.boxR) || xc >= e.boxR || t-e.pressT > 2.5) {
		e.env.MouseUp()
		e.holding = false
	}
}

// glovesNow - выбран ли слот с перчатками (нужна найденная шкала стамины).
func (e *Engine) glovesNow() GlovesState {
	w, h, ok := e.env.ClientSize()
	if !ok || !e.stamOK {
		return GlovesUnknown
	}
	return Gloves(e.env.Grab(HotbarRect(e.stam, w, h)), e.stam.Scale())
}

// bagTick - режим 3: бьёт грушу ЛКМ и ПКМ по очереди; когда стамина кончилась,
// ждёт её полного восстановления (и ещё fullWait секунд) и продолжает.
func (e *Engine) bagTick() {
	now := e.env.Now()
	if _, _, ok := e.env.ClientSize(); !ok {
		e.say("st.nowin")
		e.env.Sleep(300)
		return
	}
	if now >= e.bagCheckAt {
		interval := 0.12
		if e.bagRest {
			interval = 0.03
		}
		e.bagCheckAt = now + interval
		e.bagFr, e.bagSok = e.readStamina()
		e.bagGl = e.glovesNow()
	}
	fr, sok := e.bagFr, e.bagSok
	// перчатки на руках?
	if e.bagGl == GlovesOff {
		if !e.glvWarned {
			e.glvWarned = true
			e.env.Beep()
		}
		e.setWarn(WarnGloves, true)
		e.say("st.nogloves")
		e.env.Sleep(150)
		return
	}
	e.setWarn(WarnGloves, false)
	if e.glvWarned {
		e.glvWarned = false
		e.nextPunch = now
	}
	if now >= e.warnNext {
		e.checkWarnings(e.bagEnergy)
		e.bagEnergy = false
		e.warnNext = now + 1.5
		if e.capSeen {
			e.warnNext = now + 3600
		}
	}
	if e.bagRest {
		if sok && fr >= 0.995 {
			if e.fullSince < 0 {
				e.fullSince = now
			}
			if now-e.fullSince >= fullWait {
				e.bagRest, e.fullSince = false, -1
				e.nextPunch = now
				e.clearWarn(WarnCap | WarnEnergy)
				e.capSeen, e.bagEnergy, e.warnNext = false, true, now+0.4
				e.say("st.bag")
				return
			}
			e.say("st.full_wait")
			e.env.Sleep(30)
			return
		}
		e.fullSince = -1
		if sok {
			e.say("st.wait_stam", int(fr*100))
		} else {
			e.say("st.nostam")
		}
		e.env.Sleep(150)
		return
	}
	if sok && fr <= bagEmpty {
		e.bagRest, e.fullSince = true, -1
		e.say("st.bag_rest")
		return
	}
	if now >= e.nextPunch {
		if e.bagLeft {
			e.env.Click()
		} else {
			e.env.RClick()
		}
		atomic.AddInt64(&e.punches, 1)
		e.bagLeft = !e.bagLeft
		gap := e.set().PunchMs / 1000
		if gap < 0.05 {
			gap = 0.05
		}
		e.nextPunch += gap
		if e.nextPunch < now {
			e.nextPunch = now + gap
		}
		e.say("st.bag")
	}
	e.env.Sleep(4)
}

// refineBar заново ищет полоску, когда индикатор далеко и от центра, и от краёв
// (тогда он ничего не закрывает), и уточняет положение центра.
func (e *Engine) refineBar(xc float64) {
	w := float64(e.bar.W())
	d := math.Abs(xc - e.bar.Center())
	if d < 0.3*w || d > 0.42*w {
		return
	}
	e.needRefine = false
	cw, ch, ok := e.env.ClientSize()
	if !ok {
		return
	}
	if bar, found := FindBar(e.env.Grab(Rect{0, 0, cw, ch})); found {
		e.setBar(bar)
	}
}
