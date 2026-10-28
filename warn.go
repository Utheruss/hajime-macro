package main

// Распознавание двух предупреждений игры:
//  1. сообщение внизу экрана "<стат> is at your goal (N) - training won't raise it further"
//     (кап стата достигнут), ищется по маске надписи "training won't raise it further";
//  2. жёлтая шкала энергии (левая из трёх трубок справа внизу).

// ---------- сообщение о капе ----------

type tplPix struct{ x, y int }

var capW, capD []tplPix
var capTW, capTH int

func init() {
	capTH, capTW = len(capTpl), len(capTpl[0])
	for y, row := range capTpl {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case 'W':
				capW = append(capW, tplPix{x, y})
			case 'D':
				capD = append(capD, tplPix{x, y})
			}
		}
	}
}

func rnd(v float64) int { return int(v + 0.5) }

// CapBand - полоса кадра, где может быть сообщение о капе.
func CapBand(w, h int) Rect {
	x0, y0 := w*20/100, h*60/100
	return Rect{x0, y0, w*90/100 - x0, h*88/100 - y0}
}

// softWhite / softDark - насколько пиксель похож на белый текст / тёмную обводку (0..1);
// мягкие оценки нужны, потому что при масштабировании кадра края текста размываются.
func softWhite(f *Frame, x, y int) float64 {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return 0
	}
	i := (y*f.W + x) * 4
	m := minI(int(f.Pix[i]), minI(int(f.Pix[i+1]), int(f.Pix[i+2])))
	v := float64(m-110) / 110
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func softDark(f *Frame, x, y int) float64 {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return 0
	}
	i := (y*f.W + x) * 4
	m := maxI(int(f.Pix[i]), maxI(int(f.Pix[i+1]), int(f.Pix[i+2])))
	v := float64(150-m) / 100
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// capScore - насколько маска, масштабированная в k раз, совпадает с кадром, если её левый верхний угол в (ox,oy).
// Второй результат false, если быстрая проверка по выборке пикселей уже провалилась.
func capScore(f *Frame, ox, oy int, k float64) (float64, bool) {
	px := func(p tplPix) (int, int) {
		return ox + int((float64(p.x)+0.5)*k), oy + int((float64(p.y)+0.5)*k)
	}
	step := len(capW)/24 + 1
	miss := 0
	for i := 0; i < len(capW); i += step {
		x, y := px(capW[i])
		if softWhite(f, x, y) < 0.35 {
			miss++
			if miss > 7 {
				return 0, false
			}
		}
	}
	sw := 0.0
	for _, p := range capW {
		x, y := px(p)
		sw += softWhite(f, x, y)
	}
	sd := 0.0
	for _, p := range capD {
		x, y := px(p)
		sd += softDark(f, x, y)
	}
	sw /= float64(len(capW))
	sd /= float64(len(capD))
	if sw < sd {
		return sw, true
	}
	return sd, true
}

// capThreshold - минимальная оценка совпадения (меньшая из долей белого и обводки; на скринах без текста не выше 0.52, на масштабированных с текстом от 0.65).
const capThreshold = 0.60

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// CapMessage ищет сообщение о капе в кадре f (обычно в полосе CapBand). ks - возможные масштабы
// интерфейса (текст в игре может масштабироваться вместе с ним или нет).
func CapMessage(f *Frame, ks []float64) bool {
	for _, k := range ks {
		if k < 0.4 || k > 3 {
			continue
		}
		tw, th := rnd(float64(capTW)*k), rnd(float64(capTH)*k)
		for oy := 0; oy+th <= f.H; oy++ {
			for ox := 0; ox+tw <= f.W; ox++ {
				if sc, pass := capScore(f, ox, oy, k); pass && sc >= capThreshold {
					return true
				}
			}
		}
	}
	return false
}

// ---------- шкала энергии ----------

type EnergyGeom struct{ X0, X1, Y0, Y1 int } // внутренняя часть трубки

func isNavy(r, g, b int) bool { return r < 80 && g < 80 && b < 120 && b >= r+15 }

func isYellowFill(r, g, b int) bool { return r >= 190 && g >= 170 && b <= 200 && r-b >= 50 }

type edgeCl struct{ x0, x1, top, bot int }

// LocateEnergy ищет левую из трёх трубок (энергия, еда, вода) в правом нижнем углу.
func LocateEnergy(f *Frame) (EnergyGeom, bool) {
	xs := f.W * 68 / 100
	ys := f.H * 45 / 100
	minRun := f.H * 6 / 100
	var cls []edgeCl
	for x := xs; x < f.W; x++ {
		best, bt, bb := 0, 0, 0
		cur, ct := 0, 0
		gap := 0
		for y := ys; y < f.H; y++ {
			r, g, b := f.RGB(x, y)
			if isNavy(r, g, b) {
				if cur == 0 {
					ct = y
				}
				cur += 1 + gap
				gap = 0
				if cur > best {
					best, bt, bb = cur, ct, y
				}
			} else if cur > 0 && gap < 1 {
				gap++
			} else {
				cur, gap = 0, 0
			}
		}
		if best < minRun {
			continue
		}
		if n := len(cls); n > 0 && cls[n-1].x1 >= x-1 {
			cls[n-1].x1 = x
			continue
		}
		cls = append(cls, edgeCl{x, x, bt, bb})
	}
	// когда трубка почти пуста, её тёмная внутренность тоже даёт длинные "рёбра"
	// (между ней и бортиками остаются светлые полоски), поэтому пробуем и пару через один кластер
	for _, c := range cls {
		// вся трубка слилась в один кластер (пустая, без светлых бортиков)
		wd, run := c.x1-c.x0+1, c.bot-c.top+1
		if wd*100 >= run*17 && wd*100 <= run*30 {
			th := maxI(1, wd/13)
			if g, ok := tubeFrom(f, edgeCl{c.x0, c.x0 + th - 1, c.top, c.bot}, edgeCl{c.x1 - th + 1, c.x1, c.top, c.bot}); ok {
				return g, true
			}
		}
	}
	for i := 0; i+1 < len(cls); i++ {
		for _, j := range []int{i + 1, i + 2} {
			if j >= len(cls) {
				break
			}
			if g, ok := tubeFrom(f, cls[i], cls[j]); ok {
				return g, true
			}
		}
	}
	return EnergyGeom{}, false
}

func tubeFrom(f *Frame, a, b edgeCl) (EnergyGeom, bool) {
	{
		run := a.bot - a.top + 1
		iw := b.x0 - a.x1 - 1
		if iw*100 < run*10 || iw*100 > run*26 {
			return EnergyGeom{}, false
		}
		// трубки стоят у нижнего края экрана, высота около 12% кадра
		if a.bot < f.H*90/100 || run*100 < f.H*8 || run*100 > f.H*20 {
			return EnergyGeom{}, false
		}
		if d := a.top - b.top; d > run/10 || -d > run/10 {
			return EnergyGeom{}, false
		}
		th := a.x1 - a.x0 + 1
		g := EnergyGeom{a.x1 + 1, b.x0 - 1, a.top + th, a.bot - th}
		if g.Y1-g.Y0 < run/2 {
			return EnergyGeom{}, false
		}
		// внутри должны быть цвета заливки или пустой части трубки
		cx := g.X0 + (g.X1-g.X0)*2/5
		ok, tot := 0, 0
		for y := g.Y0; y <= g.Y1; y++ {
			r, gg, bb := f.RGB(cx, y)
			tot++
			if isYellowFill(r, gg, bb) || isNavy(r, gg, bb) {
				ok++
			}
		}
		if tot > 0 && ok*100 >= tot*85 {
			return g, true
		}
	}
	return EnergyGeom{}, false
}

// Energy - заполненность шкалы энергии 0..1 (по самой верхней жёлтой строке заливки).
func Energy(f *Frame, g EnergyGeom) float64 {
	cx := g.X0 + (g.X1-g.X0)*2/5
	top := -1
	for y := g.Y0; y <= g.Y1; y++ {
		r, gg, b := f.RGB(cx, y)
		if isYellowFill(r, gg, b) {
			top = y
			break
		}
	}
	if top < 0 {
		return 0
	}
	return float64(g.Y1-top+1) / float64(g.Y1-g.Y0+1)
}

// EnergyLow - энергии меньше 10%. ok=false, если шкала на экране не найдена.
func EnergyLow(f *Frame) (low, ok bool) {
	g, found := LocateEnergy(f)
	if !found {
		return false, false
	}
	return Energy(f, g) < 0.10, true
}

// ---------- энергия, еда, вода ----------

// Supplies - заполненность трёх шкал справа внизу (0..1).
type Supplies struct {
	Energy, Food, Water float64
	OK                  bool
}

func isFoodFill(r, g, b int) bool  { return r >= 75 && r > g && g >= b-5 && r-b >= 45 }
func isWaterFill(r, g, b int) bool { return b >= 140 && b > g && g > r && b-r >= 60 }

// tubeFill - заполненность трубки по самой верхней строке заливки в её внутренней части.
func tubeFill(f *Frame, x0, x1, y0, y1 int, fill func(r, g, b int) bool) float64 {
	if x0 < 0 || y0 < 0 || x1 >= f.W || y1 >= f.H || y1 <= y0 {
		return 0
	}
	cx := x0 + (x1-x0)*2/5
	for y := y0; y <= y1; y++ {
		r, g, b := f.RGB(cx, y)
		if fill(r, g, b) {
			return float64(y1-y+1) / float64(y1-y0+1)
		}
	}
	return 0
}

// ReadSupplies читает энергию, еду и воду. Положение еды и воды берётся от трубки энергии
// (трубки стоят рядом на одинаковом расстоянии и масштабируются вместе с интерфейсом).
func ReadSupplies(f *Frame) Supplies {
	g, ok := LocateEnergy(f)
	if !ok {
		return Supplies{}
	}
	s := float64(g.Y1-g.Y0+1) / 123.0
	off := func(v float64) int { return int(v*s + 0.5) }
	return Supplies{
		Energy: Energy(f, g),
		Food:   tubeFill(f, g.X0+off(30), g.X1+off(30), g.Y0+off(4), g.Y1-off(2), isFoodFill),
		Water:  tubeFill(f, g.X0+off(60), g.X1+off(60), g.Y0+off(2), g.Y1, isWaterFill),
		OK:     true,
	}
}

// Пороги: энергия меньше 10%, еда и вода - 5% и меньше.
const (
	energyLowAt = 0.10
	foodLowAt   = 0.055
	waterLowAt  = 0.055
)
