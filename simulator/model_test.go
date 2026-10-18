package simulator

import (
	"SlamSim/fastgen"
	"errors"
	"testing"
)

func newTestWorld(t *testing.T, seed uint64) *World {
	t.Helper()
	res := fastgen.New(seed, seed+1, fastgen.DefaultConfig()).Next()
	w, err := NewWorld(res.Pattern, res.Route, -1)
	if err != nil {
		t.Fatal(err)
	}
	if w.reference != res.LogisticsCost-10 {
		t.Fatalf("эталон %d, ожидалось %d", w.reference, res.LogisticsCost-10)
	}
	return w
}

func dirTo(a, b int) int {
	switch b - a {
	case -N:
		return north
	case N:
		return south
	case 1:
		return east
	}
	return west
}

func face(t *testing.T, w *World, d int) {
	for w.heading != d {
		if err := w.TurnRight(); err != nil {
			t.Fatal(err)
		}
	}
}

// Проезд по эталонному маршруту fastgen должен быть легальным и дать максимум баллов
func TestFollowReferenceRoute(t *testing.T) {
	for seed := uint64(0); seed < 300; seed++ {
		w := newTestWorld(t, seed*7)
		for _, seg := range w.route {
			start := 0
			if seg[0] != w.pos { // сегмент начинается с трубы/подставки, робот уже на соседней клетке
				start = 1
			}
			for i := start; i+1 < len(seg); i++ {
				face(t, w, dirTo(seg[i], seg[i+1]))
				next := seg[i+1]
				var err error
				switch {
				case isTube(w.base[next]):
					err = w.Pick(1)
				case isHolder(w.base[next]):
					err = w.Place("")
				default:
					err = w.Forward()
				}
				if err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
			}
		}
		sc := w.Score()
		if sc.Total != 55 || w.sections != w.reference {
			t.Fatalf("seed %d: баллы %+v, секции %d, эталон %d", seed, sc, w.sections, w.reference)
		}
	}
}

func TestIllegalCommandsKeepState(t *testing.T) {
	w := newTestWorld(t, 42)
	var cmdErr *CommandError
	for d := 0; d < 4; d++ {
		pos, h := w.pos, w.heading
		if err := w.Pick(1); !errors.As(err, &cmdErr) && err != nil {
			t.Fatal(err)
		}
		if err := w.Place(""); !errors.As(err, &cmdErr) {
			t.Fatalf("place без трубы должен быть ошибкой, получено %v", err)
		}
		err := w.Forward()
		if err != nil && (w.pos != pos || w.heading != h) {
			t.Fatal("нелегальная команда изменила состояние")
		}
		if err != nil && !errors.As(err, &cmdErr) {
			t.Fatalf("ожидалась CommandError, получено %T", err)
		}
		w.TurnLeft()
	}
}

func TestSensorFrame(t *testing.T) {
	w := newTestWorld(t, 5)
	w.heading = east
	c, ok := w.relative(1, 0)
	want, wok := w.neighbour(w.pos, east)
	if ok != wok || ok && c != want {
		t.Fatal("клетка «вперёд» при курсе на восток должна быть восточной")
	}
	c, ok = w.relative(0, 1)
	want, wok = w.neighbour(w.pos, south)
	if ok != wok || ok && c != want {
		t.Fatal("клетка «вправо» при курсе на восток должна быть южной")
	}
}
