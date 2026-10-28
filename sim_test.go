package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Симулятор игры: рисует полоску так же, как в игре, двигает индикатор и считает ошибки кликов.
type sim struct {
	t        float64
	limit    float64
	mode     int
	runMode  int     // что сообщаем движку: 0 - авто, 1/2 - принудительно, 3 - груша
	dispLag  float64 // задержка картинки на экране, с
	inLag    float64 // задержка ввода, с
	grabCost float64

	cw, ch             int
	bx0, bx1, by0, by1 int
	bar                bool

	// индикатор
	hist  []float64 // x индикатора через каждые 1 мс
	x     float64
	dir   float64
	speed float64
	accel bool
	maxSp float64
	stam  float64

	// режим 2
	boxL, boxR float64
	rng        *rand.Rand

	// события ввода
	pending        []evt
	downX          float64
	holding        bool
	errs           []float64 // клики режима 1: x - центр
	startE         []float64 // режим 2: x при нажатии - boxL
	endE           []float64 // x при отпускании - boxR
	clicks         int
	rclicks, beeps int
	inactive       [][2]float64 // интервалы, когда окно игры не активно
	punches        []float64
	clickTimes     []float64
	hotbar         bool
	capMsg         bool
	food, water    float64 // <0 - шкалы нет
	equip          int
	items          bool // предметы в слотах 2-4 есть
	eStops         bool // E закрывает полоску (тренировка прерывается)
	log            []evtLog
	energy         float64 // <0 - шкалы нет на экране
	glovesOn       bool
	drain          float64
	keyE           int
	stepT          float64
}

type evtLog struct {
	t    float64
	what string
}

type evt struct {
	t    float64
	kind string
}

func newSim(mode int, speed float64, accel bool) *sim {
	s := &sim{mode: mode, cw: 1917, ch: 1078, bx0: 721, bx1: 1198, by0: 406, by1: 428, bar: true,
		dispLag: 0.025, inLag: 0.010, grabCost: 0.002, limit: 30, rng: rand.New(rand.NewSource(1)),
		speed: speed, accel: accel, maxSp: 960, stam: 1, dir: 1}
	s.energy, s.food, s.water = -1, -1, -1
	s.x = float64(s.bx0) + 2
	s.newBox()
	return s
}

func (s *sim) newBox() {
	w := 100 + s.rng.Float64()*60
	l := float64(s.bx0) + 60 + s.rng.Float64()*(float64(s.bx1-s.bx0)-w-120)
	s.boxL, s.boxR = l, l+w
}

func (s *sim) center() float64 { return float64(s.bx0+s.bx1) / 2 }

// шаг физики 1 мс
func (s *sim) step() {
	dt := 0.001
	s.stepT += dt
	if s.mode == 3 {
		s.stam = math.Max(0, math.Min(1, s.stam+0.08*dt))
	}
	if s.mode == 1 {
		s.x += s.dir * s.speed * dt
		if s.x >= float64(s.bx1)-4 {
			s.x, s.dir = float64(s.bx1)-4, -1
		}
		if s.x <= float64(s.bx0)+4 {
			s.x, s.dir = float64(s.bx0)+4, 1
		}
	} else {
		s.x += s.speed * dt
		if s.x >= float64(s.bx1)-2 {
			s.x = float64(s.bx0) + 2
			s.newBox()
			s.holding = false
		}
	}
	s.hist = append(s.hist, s.x)
	// события ввода, которые доехали
	now := s.stepT
	rest := s.pending[:0]
	for _, ev := range s.pending {
		if ev.t > now {
			rest = append(rest, ev)
			continue
		}
		switch ev.kind {
		case "click":
			s.errs = append(s.errs, s.x-s.center())
			if s.accel {
				s.speed = math.Min(s.maxSp, s.speed*1.08)
			}
		case "down":
			s.startE = append(s.startE, s.x-s.boxL)
			s.downX = s.x
			s.holding = true
		case "up":
			if s.holding {
				s.endE = append(s.endE, s.x-s.boxR)
				s.holding = false
			}
		}
	}
	s.pending = rest
}

func (s *sim) advanceTo(t float64) {
	for s.stepT < t {
		s.step()
	}
	s.t = t
}

func (s *sim) xAt(t float64) float64 {
	i := int(t * 1000)
	if i < 0 {
		i = 0
	}
	if i >= len(s.hist) {
		i = len(s.hist) - 1
	}
	if i < 0 {
		return s.x
	}
	return s.hist[i]
}

func (s *sim) ClientSize() (int, int, bool) { return s.cw, s.ch, true }
func (s *sim) Now() float64                 { return s.t }
func (s *sim) Sleep(ms float64)             { s.advanceTo(s.t + ms/1000) }
func (s *sim) Done() bool                   { return s.t >= s.limit }
func (s *sim) GameActive() bool {
	for _, w := range s.inactive {
		if s.t >= w[0] && s.t < w[1] {
			return false
		}
	}
	return true
}
func (s *sim) EnsureFocus() {}
func (s *sim) KeyE() {
	s.keyE++
	s.log = append(s.log, evtLog{s.t, "E"})
	if s.eStops {
		s.bar = !s.bar // E останавливает тренировку или запускает новую
	}
}

// Key - цифровой слот: выбранный предмет можно "использовать" кликом
func (s *sim) Key(n int) {
	s.advanceTo(s.t + 0.001)
	s.equip = n
	s.log = append(s.log, evtLog{s.t, fmt.Sprint(n)})
}
func (s *sim) Click() {
	s.log = append(s.log, evtLog{s.t, "click"})
	if s.equip >= 2 && s.equip <= 4 && s.items {
		// клик с предметом в руке: пополняет соответствующую шкалу
		switch s.equip {
		case 4:
			s.energy = math.Min(1, s.energy+0.3)
		case 2:
			s.food = math.Min(1, s.food+0.3)
		case 3:
			s.water = math.Min(1, s.water+0.3)
		}
		return
	}
	s.clicks++
	s.clickTimes = append(s.clickTimes, s.t)
	if s.mode == 3 {
		s.punches = append(s.punches, s.t)
		s.stam -= s.drain
		return
	}
	s.pending = append(s.pending, evt{s.t + s.inLag, "click"})
}
func (s *sim) RClick()    { s.rclicks++; s.punches = append(s.punches, s.t); s.stam -= s.drain }
func (s *sim) Beep()      { s.beeps++ }
func (s *sim) MouseDown() { s.pending = append(s.pending, evt{s.t + s.inLag, "down"}) }
func (s *sim) MouseUp()   { s.pending = append(s.pending, evt{s.t + s.inLag, "up"}) }

func (s *sim) Grab(r Rect) *Frame {
	s.advanceTo(s.t + s.grabCost)
	ix := s.xAt(s.t - s.dispLag)
	f := NewFrame(r.W, r.H)
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			R, G, B := s.pixel(r.X+x, r.Y+y, ix)
			i := (y*r.W + x) * 4
			f.Pix[i], f.Pix[i+1], f.Pix[i+2], f.Pix[i+3] = byte(B), byte(G), byte(R), 255
		}
	}
	return f
}

const ch0 = 993

// панель слотов: у слота 1 рамка светлая, если выбран, у остальных серая
func (s *sim) hotbarPixel(x, y int) (int, int, int, bool) {
	for i := 0; i < 9; i++ {
		l := 676 + i*64
		if x >= l && x < l+58 && y >= 1008 && y < 1066 {
			edge := x < l+3 || x >= l+55 || y < 1011 || y >= 1063
			if edge {
				if i == 0 && s.glovesOn {
					return 214, 216, 224, true
				}
				return 92, 94, 102, true
			}
		}
	}
	return 0, 0, 0, false
}

// трубка энергии и сообщение о капе, как в игре (масштаб 1.0)
func (s *sim) warnPixel(x, y int) (int, int, int, bool) {
	if s.capMsg {
		tx, ty := x-993, y-793
		if ty >= 0 && ty < len(capTpl) && tx >= 0 && tx < len(capTpl[0]) {
			switch capTpl[ty][tx] {
			case 'W':
				return 255, 255, 255, true
			case 'D':
				return 14, 12, 30, true
			}
		}
	}
	if s.energy >= 0 && x >= 1814 && x <= 1839 && y >= 931 && y <= 1057 {
		if x <= 1815 || x >= 1838 || y <= 932 || y >= 1056 {
			return 19, 18, 58, true
		}
		if x <= 1817 {
			return 169, 173, 220, true
		}
		if x >= 1836 {
			return 91, 93, 148, true
		}
		top := 1055 - int(s.energy*123)
		if y > top {
			return 232, 217, 83, true
		}
		return 42, 40, 66, true
	}
	// еда (x+30) и вода (x+60): цвета как в игре
	for i, lvl := range []float64{s.food, s.water} {
		if lvl < 0 {
			continue
		}
		dx := 30 * (i + 1)
		if x < 1814+dx || x > 1839+dx || y < 924 || y > 1057 {
			continue
		}
		xx := x - dx
		if xx <= 1815 || xx >= 1838 || y <= 925 || y >= 1056 {
			return 19, 18, 58, true
		}
		if xx <= 1817 {
			return 169, 173, 220, true
		}
		if xx >= 1836 {
			return 91, 93, 148, true
		}
		if i == 0 { // еда
			if y < 937 {
				return 138, 130, 192, true
			}
			if y > 1053 {
				return 58, 45, 92, true
			}
			if y >= 1053-int(lvl*117)+1 {
				return 168, 105, 60, true
			}
			return 44, 34, 54, true
		}
		if y < 935 {
			return 63, 111, 208, true
		}
		if y >= 1055-int(lvl*121)+1 {
			return 120, 176, 212, true
		}
		return 38, 58, 86, true
	}
	return 0, 0, 0, false
}

func (s *sim) pixel(x, y int, ix float64) (int, int, int) {
	if r, g, b, ok := s.warnPixel(x, y); ok {
		return r, g, b
	}
	if s.hotbar && y >= ch0 && x >= 658+0 {
		if r, g, b, ok := s.hotbarPixel(x, y); ok {
			return r, g, b
		}
	}
	// шкала стамины
	if x >= 728 && x <= 1191 && y >= 965 && y <= 971 {
		if x == 844 || x == 960 || x == 1076 {
			return 26, 40, 60
		}
		if float64(x-728) < s.stam*464 {
			return 72, 140, 210
		}
		return 86, 84, 89
	}
	if x >= 728 && x <= 1191 && (y == 959 || y == 960) {
		return 82, 80, 86
	}
	if s.bar {
		cx := (s.bx0 + s.bx1) / 2
		if x >= cx-5 && x <= cx+5 && ((y >= s.by0-14 && y <= s.by0-3) || (y >= s.by1+3 && y <= s.by1+14)) {
			return 255, 255, 255
		}
		ixi := int(math.Round(ix))
		if y >= s.by0-17 && y <= s.by1+17 {
			if x >= ixi-3 && x <= ixi+3 {
				return 255, 255, 255
			}
			if x == ixi-4 || x == ixi+4 {
				return 30, 30, 33
			}
		}
		if s.mode == 2 && y >= s.by0+2 && y <= s.by1-2 {
			if x >= int(s.boxL) && x <= int(s.boxR) {
				return 242, 238, 228
			}
		}
		if x >= s.bx0 && x <= s.bx1 && y >= s.by0 && y <= s.by1 {
			return 198, 51, 52
		}
		if x >= s.bx0-8 && x <= s.bx1+8 && y >= s.by0-6 && y <= s.by1+6 {
			return 30, 29, 33
		}
	}
	return 200, 170, 100
}

func run(t *testing.T, s *sim, lead float64) *Engine {
	set := func() Settings { return Settings{Mode: s.runMode, LeadMs: lead, PunchMs: 300} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	return e
}

func stats(v []float64) (mean, maxAbs float64) {
	for _, x := range v {
		mean += x
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
	}
	if len(v) > 0 {
		mean /= float64(len(v))
	}
	return
}

func TestSimMode1(t *testing.T) {
	for _, sp := range []float64{160, 300, 600, 960} {
		s := newSim(1, sp, false)
		s.limit = 20
		run(t, s, 35)
		m, mx := stats(s.errs)
		t.Logf("режим1 скорость %4.0f px/с: кликов %d, средняя ошибка %+.1f px, макс %.1f px", sp, len(s.errs), m, mx)
		if len(s.errs) < 5 || mx > 14 {
			t.Errorf("слишком плохо при скорости %v", sp)
		}
	}
	s := newSim(1, 160, true)
	s.limit = 40
	run(t, s, 35)
	m, mx := stats(s.errs)
	t.Logf("режим1 ускоряющаяся (punch speed): кликов %d, средняя %+.1f px, макс %.1f px, конечная скорость %.0f", len(s.errs), m, mx, s.speed)
	if mx > 20 {
		t.Errorf("punch speed: макс ошибка %.1f", mx)
	}
}

func TestSimMode2(t *testing.T) {
	for _, sp := range []float64{160, 160 * 1.5} {
		s := newSim(2, sp, false)
		s.limit = 30
		run(t, s, 35)
		m1, x1 := stats(s.startE)
		m2, x2 := stats(s.endE)
		t.Logf("режим2 скорость %3.0f: раундов нажато %d/%d, начало %+.1f px (макс %.1f), конец %+.1f px (макс %.1f)", sp, len(s.startE), len(s.endE), m1, x1, m2, x2)
		if len(s.startE) < 3 || x1 > 14 || x2 > 14 {
			t.Errorf("режим 2 плохо при скорости %v", sp)
		}
	}
}

func TestSimEndsAndRestart(t *testing.T) {
	s := newSim(1, 300, false)
	s.limit = 10
	s.bar = false
	s.stam = 0.2
	e := run(t, s, 35)
	if s.keyE != 0 {
		t.Errorf("E нажали при стамине 20%%")
	}
	_ = e
	s2 := newSim(1, 300, false)
	s2.limit = 3
	s2.bar = false
	s2.stam = 1
	run(t, s2, 35)
	if s2.keyE == 0 {
		t.Errorf("E не нажали при полной стамине")
	}
}

func TestSimBag(t *testing.T) {
	s := newSim(3, 0, false)
	s.bar = false
	s.hotbar, s.glovesOn = true, true
	s.limit = 60
	s.drain = 0.06
	// стамина понемногу восстанавливается сама, как в игре
	set := func() Settings { return Settings{Mode: 3, LeadMs: 35, PunchMs: 300} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	n := len(s.punches)
	gaps := 0
	for i := 1; i < n; i++ {
		if s.punches[i]-s.punches[i-1] > 3 {
			gaps++
		}
	}
	t.Logf("пауз на восстановление: %d", gaps)
	if gaps < 2 {
		t.Errorf("ожидали минимум 2 паузы на восстановление стамины, было %d", gaps)
	}
	t.Logf("груша: ударов %d (ЛКМ %d, ПКМ %d), писков %d, стамина в конце %.2f", n, s.clicks, s.rclicks, s.beeps, s.stam)
	if s.clicks == 0 || s.rclicks == 0 || abs2(s.clicks-s.rclicks) > 1 {
		t.Errorf("ЛКМ и ПКМ должны чередоваться")
	}
	for i := 1; i < n; i++ {
		if d := s.punches[i] - s.punches[i-1]; d < 0.28 && d > 0 {
			t.Errorf("слишком частые удары: %.3f", d)
			break
		}
	}
}

func abs2(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func TestSimGlovesWarning(t *testing.T) {
	s := newSim(3, 0, false)
	s.bar = false
	s.hotbar, s.glovesOn = true, false
	s.limit = 3
	set := func() Settings { return Settings{Mode: 3, LeadMs: 35, PunchMs: 300} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	if s.clicks+s.rclicks != 0 {
		t.Errorf("без перчаток бить нельзя, ударов %d", s.clicks+s.rclicks)
	}
	if s.beeps != 1 {
		t.Errorf("ожидали ровно один писк, было %d", s.beeps)
	}
}

func TestSimPause(t *testing.T) {
	s := newSim(1, 300, false)
	s.limit = 14
	s.inactive = [][2]float64{{5, 9}}
	run(t, s, 35)
	before, during, after := 0, 0, 0
	for _, c := range s.clickTimes {
		switch {
		case c < 5:
			before++
		case c < 9.05:
			during++
		default:
			after++
		}
	}
	t.Logf("клики до паузы %d, во время паузы %d, после %d", before, during, after)
	if before < 3 || during != 0 || after < 3 {
		t.Errorf("пауза без фокуса работает неправильно")
	}
}

func TestSimAutoMode(t *testing.T) {
	// режим 0 (авто): и "клики", и "зажатие" определяются по виду полоски
	s1 := newSim(1, 300, false)
	s1.limit = 10
	e1 := run(t, s1, 35)
	if e1.curMode != 1 || len(s1.errs) < 5 {
		t.Errorf("полоска-клики: режим %d, кликов %d", e1.curMode, len(s1.errs))
	}
	s2 := newSim(2, 160, false)
	s2.limit = 12
	e2 := run(t, s2, 35)
	if e2.curMode != 2 || len(s2.startE) < 2 {
		t.Errorf("полоска-зажатие: режим %d, нажатий %d", e2.curMode, len(s2.startE))
	}
}

func TestSimGlovesWarningBar(t *testing.T) {
	// Punch speed: без перчаток E не нажимается, писк один раз
	s := newSim(1, 0, false)
	s.bar = false
	s.hotbar, s.glovesOn = true, false
	s.limit = 3
	set := func() Settings { return Settings{Mode: 1, LeadMs: 35, PunchMs: 300, Gloves: true} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	if s.beeps != 1 {
		t.Errorf("ожидали один писк, было %d", s.beeps)
	}
	if s.keyE != 0 {
		t.Errorf("без перчаток E нажимать нельзя, нажато %d", s.keyE)
	}
}

func TestSimWarnings(t *testing.T) {
	s := newSim(1, 300, false)
	s.limit = 6
	s.capMsg, s.energy = true, 0.06
	e := run(t, s, 35)
	if w := e.Warnings(); w&WarnCap == 0 || w&WarnEnergy == 0 {
		t.Errorf("ожидали предупреждения о капе и энергии, флаги %b", w)
	}
	if s.beeps != 1 {
		t.Errorf("ожидали один писк на обе новые тревоги, было %d", s.beeps)
	}
	// новая тренировка: предупреждения сбрасываются и появляются снова со звуком
	e.newTrainingWarnings(e.env.Now())
	if s.beeps != 2 || e.Warnings()&(WarnCap|WarnEnergy) != WarnCap|WarnEnergy {
		t.Errorf("при новой тренировке сигнал должен повториться: писков %d, флаги %b", s.beeps, e.Warnings())
	}
	// энергию пополнили, кап убрали - тревог нет
	s.capMsg, s.energy = false, 0.8
	e.newTrainingWarnings(e.env.Now())
	if e.Warnings() != 0 || s.beeps != 2 {
		t.Errorf("тревог быть не должно: флаги %b, писков %d", e.Warnings(), s.beeps)
	}
}

func TestSimNoWarnings(t *testing.T) {
	s := newSim(1, 300, false)
	s.limit = 4
	s.energy = 0.9
	e := run(t, s, 35)
	if e.Warnings() != 0 || s.beeps != 0 {
		t.Errorf("тревог быть не должно: флаги %b, писков %d", e.Warnings(), s.beeps)
	}
}

func TestSimBagWarnings(t *testing.T) {
	s := newSim(3, 0, false)
	s.bar = false
	s.hotbar, s.glovesOn = true, true
	s.limit = 6
	s.capMsg, s.energy = true, 0.05
	set := func() Settings { return Settings{Mode: 3, LeadMs: 35, PunchMs: 300} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	if w := e.Warnings(); w&WarnCap == 0 || w&WarnEnergy == 0 {
		t.Errorf("груша: ожидали предупреждения о капе и энергии, флаги %b", w)
	}
	if s.beeps != 1 {
		t.Errorf("груша: ожидали один писк, было %d", s.beeps)
	}
}

// lockedEnv - обёртка, делающая симулятор безопасным для вызовов из фонового потока.
type lockedEnv struct {
	*sim
	mu *sync.Mutex
}

func (l lockedEnv) Grab(r Rect) *Frame {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sim.Grab(r)
}

func (l lockedEnv) Beep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sim.Beep()
}

func TestSimWarningsAsync(t *testing.T) {
	s := newSim(1, 300, false)
	s.capMsg, s.energy = true, 0.06
	var mu sync.Mutex
	env := lockedEnv{s, &mu}
	e := NewEngine(env, func() Settings { return Settings{Mode: 1, LeadMs: 35, PunchMs: 300} }, func(string, []any, float64) {})
	e.Async = true
	e.newTrainingWarnings(0)
	for i := 0; i < 400 && atomic.LoadInt32(&e.warnBusy) == 1; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	beeps := s.beeps
	mu.Unlock()
	if w := e.Warnings(); w&WarnCap == 0 || w&WarnEnergy == 0 || beeps != 1 {
		t.Errorf("async: флаги %b, писков %d", w, beeps)
	}
	// повторный запрос во время/после работы не теряется
	e.newTrainingWarnings(1)
	for i := 0; i < 400 && atomic.LoadInt32(&e.warnBusy) == 1; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	beeps = s.beeps
	mu.Unlock()
	if beeps != 2 {
		t.Errorf("async: при новой тренировке ожидали 2 писка, было %d", beeps)
	}
}

func logWhat(s *sim) []string {
	var r []string
	for _, l := range s.log {
		r = append(r, l.what)
	}
	return r
}

func supplySim(energy, food, water float64, items bool) *sim {
	s := newSim(1, 300, false)
	s.eStops, s.items = true, items
	s.energy, s.food, s.water = energy, food, water
	s.hotbar, s.glovesOn = true, true
	s.limit = 30
	return s
}

func runAuto(s *sim, auto uint32, gloves bool) *Engine {
	set := func() Settings { return Settings{Mode: 1, LeadMs: 35, PunchMs: 300, Auto: auto, Gloves: gloves} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	return e
}

func seqIndex(seq []string, want ...string) int {
	// индекс начала подпоследовательности want в seq (подряд идущие элементы), -1 - нет
	for i := 0; i+len(want) <= len(seq); i++ {
		ok := true
		for j := range want {
			if seq[i+j] != want[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func gap(t *testing.T, s *sim, from int, a, b string, want float64) {
	t.Helper()
	for i := from; i+1 < len(s.log); i++ {
		if s.log[i].what == a && s.log[i+1].what == b {
			if d := s.log[i+1].t - s.log[i].t; d < want-0.03 || d > want+0.08 {
				t.Errorf("пауза %s -> %s: %.3f с, ожидали %.1f", a, b, d, want)
			}
			return
		}
	}
}

func TestSimAutoEnergy(t *testing.T) {
	s := supplySim(0.05, 0.8, 0.8, true)
	e := runAuto(s, AutoEnergy|AutoFood|AutoWater, false)
	seq := logWhat(s)
	i := seqIndex(seq, "E", "4", "click", "click")
	if i < 0 {
		t.Fatalf("ожидали E, 4, ЛКМ, ЛКМ; было %v", seq)
	}
	gap(t, s, i, "4", "click", 0.5)
	gap(t, s, i+2, "click", "click", 0.5)
	// после пополнения тренировка начинается заново (E)
	if seqIndex(seq[i+4:], "E") < 0 {
		t.Errorf("после пополнения ожидали новый запуск тренировки (E): %v", seq)
	}
	if s.energy < 0.5 {
		t.Errorf("энергия не пополнилась: %.2f", s.energy)
	}
	_ = e
}

func TestSimAutoFoodWater(t *testing.T) {
	s := supplySim(0.9, 0.03, 0.04, true)
	runAuto(s, AutoEnergy|AutoFood|AutoWater, false)
	seq := logWhat(s)
	if i := seqIndex(seq, "E", "2", "click", "click", "click", "3", "click", "click"); i < 0 {
		t.Fatalf("ожидали E, 2, 3xЛКМ, 3, 2xЛКМ; было %v", seq)
	}
	if s.beeps != 0 {
		t.Errorf("для еды и воды сигнала быть не должно, было %d", s.beeps)
	}
}

func TestSimAutoDisabled(t *testing.T) {
	s := supplySim(0.05, 0.02, 0.02, true)
	s.limit = 8
	e := runAuto(s, 0, false)
	for _, w := range logWhat(s) {
		if w == "2" || w == "3" || w == "4" {
			t.Errorf("автопополнение выключено, а нажат слот %s", w)
		}
	}
	if e.Warnings()&WarnEnergy == 0 {
		t.Errorf("предупреждение об энергии должно оставаться")
	}
}

func TestSimAutoGlovesBack(t *testing.T) {
	s := supplySim(0.05, 0.8, 0.8, true)
	runAuto(s, AutoEnergy, true)
	if seqIndex(logWhat(s), "4", "click", "click", "1") < 0 {
		t.Errorf("после пополнения должны быть снова надеты перчатки (1): %v", logWhat(s))
	}
}

func TestSimAutoNoItems(t *testing.T) {
	// предметов нет: пополнение не помогает, повторно не пытаемся, предупреждаем
	s := supplySim(0.8, 0.02, 0.8, false)
	s.limit = 100
	e := runAuto(s, AutoFood, false)
	n := 0
	for _, w := range logWhat(s) {
		if w == "2" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("слот 2 должен быть нажат один раз, было %d", n)
	}
	if e.Warnings()&WarnSupply == 0 {
		t.Errorf("ожидали предупреждение, что пополнение не помогло")
	}
}

func TestSimCapStop(t *testing.T) {
	s := supplySim(0.9, 0.9, 0.9, true)
	s.capMsg = true
	s.limit = 15
	e := runAuto(s, AutoEnergy|AutoFood|AutoWater, false)
	if e.Running() {
		t.Errorf("при капе макрос должен остановиться")
	}
	if e.Warnings()&WarnCap == 0 {
		t.Errorf("предупреждение о капе должно остаться на экране")
	}
	if s.keyE != 1 {
		t.Errorf("тренировка должна быть прервана клавишей E один раз, было %d", s.keyE)
	}
}

func TestSimCapStopBag(t *testing.T) {
	s := newSim(3, 0, false)
	s.bar = false
	s.hotbar, s.glovesOn = true, true
	s.capMsg = true
	s.limit = 15
	set := func() Settings { return Settings{Mode: 3, LeadMs: 35, PunchMs: 300} }
	e := NewEngine(s, set, func(string, []any, float64) {})
	e.Send(CmdToggle)
	e.Run()
	if e.Running() || s.keyE != 0 {
		t.Errorf("груша: ожидали остановку без E, running=%v E=%d", e.Running(), s.keyE)
	}
}
