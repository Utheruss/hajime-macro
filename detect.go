package main

import "sort"

// Frame - кадр в формате BGRA (как отдаёт Windows GDI).
type Frame struct {
	W, H int
	Pix  []byte
}

func NewFrame(w, h int) *Frame { return &Frame{W: w, H: h, Pix: make([]byte, w*h*4)} }

func (f *Frame) RGB(x, y int) (r, g, b int) {
	i := (y*f.W + x) * 4
	return int(f.Pix[i+2]), int(f.Pix[i+1]), int(f.Pix[i])
}

// Rect - прямоугольник в координатах клиентской области игры.
type Rect struct{ X, Y, W, H int }

// Bar - заливка полоски тайминга (внутренняя цветная часть), координаты включительно.
type Bar struct{ X0, X1, Y0, Y1 int }

func (b Bar) Center() float64 { return float64(b.X0+b.X1) / 2 }
func (b Bar) H() int          { return b.Y1 - b.Y0 + 1 }
func (b Bar) W() int          { return b.X1 - b.X0 + 1 }

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func luma(r, g, b int) int { return (r*30 + g*59 + b*11) / 100 }

// family: 1 красный, 2 зелёный, 3 синий, 0 - не заливка полоски
func family(r, g, b int) int {
	switch {
	case r-maxi(g, b) >= 90:
		return 1
	case g-maxi(r, b) >= 60:
		return 2
	case b-maxi(r, g) >= 70:
		return 3
	}
	return 0
}

func isWhite(r, g, b int) bool { return r >= 240 && g >= 240 && b >= 240 }

// FindBar ищет полоску тайминга по кадру. Работает при любом размере окна:
// размеры не зашиты, а определяются по самому кадру.
//
// Заливка полоски - ровный цвет почти на всю её ширину, по краям тёмная
// рамка, а над и под серединой стоят белые треугольники (метки центра).
// Ковёр, одежда и прочие шумные цвета под это не подходят.
func FindBar(f *Frame) (Bar, bool) {
	type rowHit struct{ y, x0, x1 int }
	var hits []rowHit
	counts := make(map[uint32]int32, 256)
	firsts := make(map[uint32]int32, 256)
	lasts := make(map[uint32]int32, 256)
	minW := maxi(160, f.W/12)
	maxW := f.W * 4 / 5
	maxY := f.H * 3 / 4 // таймингa-полоска всегда в верхней части экрана, HUD внизу не трогаем
	for y := 0; y < maxY; y++ {
		for k := range counts {
			delete(counts, k)
			delete(firsts, k)
			delete(lasts, k)
		}
		base := y * f.W * 4
		for x := 0; x < f.W; x++ {
			i := base + x*4
			b, g, r := int(f.Pix[i]), int(f.Pix[i+1]), int(f.Pix[i+2])
			if family(r, g, b) == 0 {
				continue
			}
			key := uint32(r>>2)<<16 | uint32(g>>2)<<8 | uint32(b>>2)
			if counts[key] == 0 {
				firsts[key] = int32(x)
			}
			counts[key]++
			lasts[key] = int32(x)
		}
		var bestKey uint32
		var best int32
		for k, c := range counts {
			if c > best {
				best, bestKey = c, k
			}
		}
		x0, x1 := int(firsts[bestKey]), int(lasts[bestKey])
		w := x1 - x0 + 1
		if w < minW || w > maxW || int(best) < w*45/100 {
			continue
		}
		edge := maxi(8, w/40)
		if x0 < edge || x1+edge >= f.W {
			continue
		}
		okL, okR := false, false
		for d := 2; d <= edge; d++ {
			r, g, b := f.RGB(x0-d, y)
			if luma(r, g, b) < 115 {
				okL = true
			}
			r, g, b = f.RGB(x1+d, y)
			if luma(r, g, b) < 115 {
				okR = true
			}
		}
		if okL && okR {
			hits = append(hits, rowHit{y, x0, x1})
		}
	}
	// группы подряд идущих строк с одинаковым положением
	type group struct{ i, n int }
	var groups []group
	for i := 0; i < len(hits); {
		j := i + 1
		tol := maxi(8, (hits[i].x1-hits[i].x0)/60)
		for j < len(hits) && hits[j].y == hits[j-1].y+1 && abs(hits[j].x0-hits[i].x0) <= tol && abs(hits[j].x1-hits[i].x1) <= tol {
			j++
		}
		if j-i >= 6 {
			groups = append(groups, group{i, j - i})
		}
		i = j
	}
	sort.Slice(groups, func(a, b int) bool { return groups[a].n > groups[b].n })
	for _, gr := range groups {
		g := hits[gr.i : gr.i+gr.n]
		xs0 := make([]int, len(g))
		xs1 := make([]int, len(g))
		for i, h := range g {
			xs0[i], xs1[i] = h.x0, h.x1
		}
		sort.Ints(xs0)
		sort.Ints(xs1)
		bar := Bar{X0: xs0[len(g)/2], X1: xs1[len(g)/2], Y0: g[0].y, Y1: g[len(g)-1].y}
		if hasCenterMarks(f, bar) {
			return bar, true
		}
	}
	return Bar{}, false
}

// hasCenterMarks проверяет белые треугольники-метки над и под серединой полоски.
func hasCenterMarks(f *Frame, b Bar) bool {
	cx := (b.X0 + b.X1) / 2
	half := maxi(4, b.W()/80)
	n := 0
	h := b.H()
	for _, rng := range [][2]int{{b.Y0 - h, b.Y0 - 1}, {b.Y1 + 1, b.Y1 + h}} {
		for y := maxi(rng[0], 0); y <= mini(rng[1], f.H-1); y++ {
			for x := cx - half; x <= cx+half; x++ {
				if x < 0 || x >= f.W {
					continue
				}
				r, g, bl := f.RGB(x, y)
				if isWhite(r, g, bl) {
					n++
				}
			}
		}
	}
	return n >= 12
}

// stripMargins - запас вокруг полоски в кадре-полосе: по бокам, чтобы видеть
// индикатор у краёв, и сверху/снизу, где индикатор выступает за заливку.
func stripMargins(b Bar) (mx, my int) {
	return maxi(20, b.W()/24), maxi(24, b.H()*6/5+4)
}

// StripRect - узкая полоса вокруг найденной полоски, которую читаем в цикле.
func StripRect(b Bar) Rect {
	mx, my := stripMargins(b)
	return Rect{X: b.X0 - mx, Y: b.Y0 - my, W: b.W() + 2*mx, H: b.H() + 2*my}
}

// LocalBar - координаты полоски внутри кадра-полосы.
func LocalBar(b Bar) Bar {
	mx, my := stripMargins(b)
	return Bar{X0: mx, X1: mx + b.W() - 1, Y0: my, Y1: my + b.H() - 1}
}

// FindIndicator ищет индикатор - белая вертикальная палочка, идущая через всю
// высоту заливки. b - координаты полоски В ЭТОМ кадре.
func FindIndicator(f *Frame, b Bar) (float64, bool) {
	need := b.H() * 85 / 100
	maxN := maxi(22, b.W()/20)
	var cols []int
	for x := 0; x < f.W; x++ {
		n := 0
		for y := b.Y0; y <= b.Y1; y++ {
			r, g, bl := f.RGB(x, y)
			if isWhite(r, g, bl) {
				n++
			}
		}
		if n >= need {
			cols = append(cols, x)
		}
	}
	bestSum, bestN := 0.0, 0
	for i := 0; i < len(cols); {
		j := i + 1
		for j < len(cols) && cols[j]-cols[j-1] <= 2 {
			j++
		}
		n := j - i
		if n >= 2 && n <= maxN && n > bestN {
			s := 0.0
			for _, c := range cols[i:j] {
				s += float64(c)
			}
			bestSum, bestN = s, n
		}
		i = j
	}
	if bestN == 0 {
		return 0, false
	}
	return bestSum / float64(bestN), true
}

// FindBox ищет светлый бар (кремовая плашка "HOLD LEFT CLICK") внутри полоски.
// Индикатор поверх плашки белый, а не кремовый, поэтому дырки в нём перекрываем.
func FindBox(f *Frame, b Bar) (l, r int, ok bool) {
	need := b.H() * 35 / 100
	minW, maxW := b.W()*8/100, b.W()*65/100
	gap := maxi(16, b.W()/30)
	var cols []int
	for x := b.X0; x <= b.X1; x++ {
		n := 0
		for y := b.Y0; y <= b.Y1; y++ {
			rr, gg, bb := f.RGB(x, y)
			if abs(rr-242) <= 8 && abs(gg-238) <= 8 && abs(bb-228) <= 10 {
				n++
			}
		}
		if n >= need {
			cols = append(cols, x)
		}
	}
	bestW := 0
	for i := 0; i < len(cols); {
		j := i + 1
		for j < len(cols) && cols[j]-cols[j-1] <= gap {
			j++
		}
		w := cols[j-1] - cols[i] + 1
		if w >= minW && w <= maxW && w > bestW {
			bestW, l, r = w, cols[i], cols[j-1]
		}
		i = j
	}
	return l, r, bestW > 0
}

// ---------- стамина и панель слотов ----------

func isStamBlue(r, g, b int) bool { return abs(r-72) <= 3 && abs(g-140) <= 3 && abs(b-210) <= 3 }
func isStamGray(r, g, b int) bool { return abs(r-86) <= 2 && abs(g-84) <= 2 && abs(b-89) <= 2 }

// StamGeom - где на экране шкала стамины (по ней же считается масштаб интерфейса).
type StamGeom struct{ X0, X1, Y int }

func (g StamGeom) Scale() float64 { return float64(g.X1-g.X0+1) / 464.0 }

// LocateStamina ищет шкалу стамины в нижней части кадра. Положение и размер
// не зашиты, поэтому работает при любом разрешении и масштабе интерфейса.
func LocateStamina(f *Frame) (StamGeom, bool) {
	bestN, bestY, bx0, bx1 := 0, -1, 0, 0
	minN := maxi(150, f.W/10)
	for y := f.H * 55 / 100; y < f.H; y++ {
		// ищем самую длинную сплошную полосу цветных пикселей (деления шкалы - щели до 10 px)
		cn, cx0, last := 0, -1, -100
		for x := 0; x < f.W; x++ {
			r, g, b := f.RGB(x, y)
			if !(isStamBlue(r, g, b) || isStamGray(r, g, b)) {
				continue
			}
			if x-last > 10 {
				cn, cx0 = 0, x
			}
			cn++
			last = x
			if cn > bestN && cn >= minN {
				bestN, bestY, bx0, bx1 = cn, y, cx0, x
			}
		}
	}
	if bestY < 0 {
		return StamGeom{}, false
	}
	return StamGeom{X0: bx0, X1: bx1, Y: bestY}, true
}

// StaminaRect - полоса кадра вокруг шкалы стамины.
func StaminaRect(g StamGeom) Rect {
	return Rect{X: g.X0 - 8, Y: g.Y - 8, W: g.X1 - g.X0 + 17, H: 17}
}

// Stamina читает шкалу стамины из кадра StaminaRect. Возвращает долю 0..1.
func Stamina(f *Frame) (float64, bool) {
	bestN, bestB := 0, 0
	for y := 0; y < f.H; y++ {
		nb, ng := 0, 0
		for x := 0; x < f.W; x++ {
			r, g, b := f.RGB(x, y)
			if isStamBlue(r, g, b) {
				nb++
			} else if isStamGray(r, g, b) {
				ng++
			}
		}
		if nb+ng > bestN {
			bestN, bestB = nb+ng, nb
		}
	}
	if bestN < f.W/2 {
		return 0, false
	}
	return float64(bestB) / float64(bestN), true
}

// HotbarRect - панель слотов под шкалой стамины (слот 1 слева), размеры по масштабу.
func HotbarRect(g StamGeom, cw, ch int) Rect {
	k := g.Scale()
	cx := (g.X0 + g.X1) / 2
	r := Rect{X: cx - int(300*k), Y: g.Y + int(20*k), W: int(600 * k), H: int(90 * k)}
	if r.X < 0 {
		r.W += r.X
		r.X = 0
	}
	if r.X+r.W > cw {
		r.W = cw - r.X
	}
	if r.Y+r.H > ch {
		r.H = ch - r.Y
	}
	return r
}

type GlovesState int

const (
	GlovesUnknown GlovesState = iota // панели слотов не видно
	GlovesOn
	GlovesOff
)

// Gloves определяет, выбран ли слот 1 (в нём перчатки): у выбранного слота
// светлая рамка, у остальных - серая. k - масштаб интерфейса.
func Gloves(f *Frame, k float64) GlovesState {
	visible, sel := 0, 0
	lim := int(95 * k)
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			r, g, b := f.RGB(x, y)
			if abs(r-92) <= 12 && abs(g-94) <= 12 && abs(b-102) <= 12 && b-r >= 4 && b-r <= 16 {
				visible++
			} else if x <= lim && r >= 196 && r <= 232 && g >= 198 && g <= 234 && b >= 206 && b <= 242 && b-r >= 5 && b-r <= 16 {
				sel++
			}
		}
	}
	switch {
	case sel >= int(60*k*k):
		return GlovesOn
	case visible >= int(300*k*k):
		return GlovesOff
	}
	return GlovesUnknown
}
