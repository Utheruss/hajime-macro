package main

import (
	"fmt"
	"testing"
	"time"
)

func capScan(f *Frame, ks []float64) bool {
	b := CapBand(f.W, f.H)
	return CapMessage(cropFrame(f, b), ks)
}

func TestWarnDetect(t *testing.T) {
	for _, k := range []float64{0.70, 0.85, 1.00, 1.15, 1.33, 1.50} {
		path := fmt.Sprintf("/tmp/scaled2/cap_%.2f.png", k)
		f := load(t, path)
		t0 := time.Now()
		cap := capScan(f, []float64{k})
		dt := time.Since(t0)
		g, ok := LocateEnergy(f)
		fr := -1.0
		if ok {
			fr = Energy(f, g)
		}
		t.Logf("k=%.2f cap=%v (%v) energy ok=%v %+v frac=%.3f", k, cap, dt, ok, g, fr)
		if !cap {
			t.Errorf("k=%.2f: сообщение о капе не найдено", k)
		}
		if !ok || !near(fr, 0.862, 0.05) {
			t.Errorf("k=%.2f: энергия %.3f, ожидали ~0.862", k, fr)
		}
	}
}

// на скринах без сообщения ложных срабатываний быть не должно, а энергию надо находить.
func TestWarnNegatives(t *testing.T) {
	for _, s := range shots {
		f := load(t, s.file)
		if capScan(f, []float64{1.0, float64(f.H) / 1078}) {
			t.Errorf("%s: ложное срабатывание сообщения о капе", s.name)
		}
		g, ok := LocateEnergy(f)
		fr := -1.0
		if ok {
			fr = Energy(f, g)
		}
		t.Logf("%s: energy ok=%v frac=%.3f", s.name, ok, fr)
		if !ok {
			t.Errorf("%s: шкала энергии не найдена", s.name)
		}
	}
}

func TestEnergyLowSynthetic(t *testing.T) {
	for _, borders := range []bool{true, false} {
		bordL, bordR := 0, 0
		if borders {
			bordL, bordR = 2, 2
		}
		for _, tc := range []struct {
			frac float64
			low  bool
		}{{0.0, true}, {0.06, true}, {0.14, false}, {0.5, false}} {
			f := load(t, "/tmp/scaled2/cap_1.00.png")
			g, ok := LocateEnergy(f)
			if !ok {
				t.Fatal("шкала не найдена")
			}
			hgt := g.Y1 - g.Y0 + 1
			cut := g.Y1 - int(float64(hgt)*tc.frac) // выше cut - пусто
			for y := g.Y0; y <= cut; y++ {
				for x := g.X0 + bordL; x <= g.X1-bordR; x++ {
					i := (y*f.W + x) * 4
					f.Pix[i], f.Pix[i+1], f.Pix[i+2] = 66, 40, 42 // BGRA
				}
			}
			low, ok := EnergyLow(f)
			if !ok || low != tc.low {
				t.Errorf("энергия %.2f: low=%v ok=%v, ожидали low=%v", tc.frac, low, ok, tc.low)
			}
		}
	}
}

func TestSupplies(t *testing.T) {
	for _, k := range []float64{0.70, 0.85, 1.00, 1.15, 1.33, 1.50} {
		f := load(t, fmt.Sprintf("/tmp/scaled2/cap_%.2f.png", k))
		s := ReadSupplies(f)
		t.Logf("k=%.2f %+v", k, s)
		if !s.OK || !near(s.Energy, 0.862, 0.05) || !near(s.Food, 0.855, 0.05) || !near(s.Water, 0.86, 0.05) {
			t.Errorf("k=%.2f: неверные значения %+v", k, s)
		}
	}
	for _, sh := range shots {
		f := load(t, sh.file)
		s := ReadSupplies(f)
		t.Logf("%s: %+v", sh.name, s)
		if !s.OK || s.Food <= foodLowAt || s.Water <= waterLowAt {
			t.Errorf("%s: еда/вода ложно низкие %+v", sh.name, s)
		}
	}
}
