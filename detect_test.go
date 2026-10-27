package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, name string) *Frame {
	path := "/mnt/user-data/uploads/" + name
	if strings.HasPrefix(name, "/") {
		path = name
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Skip("test screenshots not available: ", path)
	}
	defer fh.Close()
	img, _, err := image.Decode(fh)
	if err != nil {
		t.Fatal(err)
	}
	bd := img.Bounds()
	f := NewFrame(bd.Dx(), bd.Dy())
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			r, g, b, _ := img.At(bd.Min.X+x, bd.Min.Y+y).RGBA()
			i := (y*f.W + x) * 4
			f.Pix[i], f.Pix[i+1], f.Pix[i+2], f.Pix[i+3] = byte(b>>8), byte(g>>8), byte(r>>8), 255
		}
	}
	return f
}

func sub(f *Frame, r Rect) *Frame { return cropFrame(f, r) }

var shots = []struct {
	name, file string
	hasBar     bool
	box        bool
	gloves     GlovesState
	stam       float64 // ожидаемая стамина, -1 - не проверять
}{
	{"arm_ready", "1793118399397_image.png", true, false, GlovesUnknown, 0.275},
	{"arm_struggle", "1793118528852_image.png", true, false, GlovesUnknown, 0.099},
	{"stopped", "1793118587657_image.png", false, false, GlovesOff, 0.011},
	{"full_stam", "1793118920110_image.png", false, false, GlovesOff, 1.0},
	{"half_stam", "1793118956787_image.png", false, false, GlovesOff, 0.48},
	{"m2_red", "1793119251038_image.png", true, true, GlovesUnknown, 0.407},
	{"torso", "1793120755192_image.png", true, true, GlovesUnknown, 0.733},
	{"jump", "1793120770883_image.png", true, false, GlovesUnknown, 1.0},
	{"punch", "1793120863645_image.png", true, false, GlovesOn, 0.758},
}

func near(a, b, tol float64) bool { return a-b <= tol && b-a <= tol }

// checkFrame прогоняет все детекторы по кадру и сверяет с ожиданиями.
func checkFrame(t *testing.T, label string, f *Frame, sc float64, hasBar, box bool, gl GlovesState, stam float64) {
	t0 := time.Now()
	bar, ok := FindBar(f)
	dt := time.Since(t0)
	if ok != hasBar {
		t.Errorf("%s: полоска найдена=%v, ожидали %v", label, ok, hasBar)
	}
	line := ""
	if ok {
		sr := StripRect(bar)
		st := sub(f, sr)
		lb := LocalBar(bar)
		x, iok := FindIndicator(st, lb)
		_, _, bok := FindBox(st, lb)
		if !iok {
			t.Errorf("%s: индикатор не найден", label)
		}
		if bok != box {
			t.Errorf("%s: белый бар найден=%v, ожидали %v", label, bok, box)
		}
		line = " индикатор x=" + ftoa(x+float64(sr.X)) + " центр=" + ftoa(bar.Center())
	}
	g, sok := LocateStamina(f)
	if !sok {
		t.Errorf("%s: шкала стамины не найдена", label)
		return
	}
	fr, rok := Stamina(sub(f, StaminaRect(g)))
	if !rok || (stam >= 0 && !near(fr, stam, 0.03)) {
		t.Errorf("%s: стамина %.3f (ok=%v), ожидали %.3f", label, fr, rok, stam)
	}
	if got := Gloves(sub(f, HotbarRect(g, f.W, f.H)), g.Scale()); got != gl {
		t.Errorf("%s: перчатки %v, ожидали %v", label, got, gl)
	}
	t.Logf("%-22s %4dx%-4d масштаб %.2f бар=%v%s стамина=%.2f перчатки=%v (поиск %dмс)", label, f.W, f.H, g.Scale(), ok, line, fr, gl, dt.Milliseconds())
}

func ftoa(v float64) string { return fmt.Sprintf("%.1f", v) }

func TestDetect(t *testing.T) {
	for _, s := range shots {
		checkFrame(t, s.name, load(t, s.file), 1, s.hasBar, s.box, s.gloves, s.stam)
	}
}

// Те же скрины, но в другом масштабе (как будто у игрока другое разрешение).
// Файлы подготавливает scripts/make_scaled.py в /tmp/scaled.
func TestDetectScaled(t *testing.T) {
	for _, tag := range []string{"n0.70", "n1.33", "l0.85", "l1.30"} {
		for _, s := range shots {
			p := "/tmp/scaled/" + tag + "_" + s.file
			if _, err := os.Stat(p); err != nil {
				t.Skip("нет масштабированных скринов")
			}
			checkFrame(t, tag+" "+s.name, load(t, p), 1, s.hasBar, s.box, s.gloves, s.stam)
		}
	}
}
