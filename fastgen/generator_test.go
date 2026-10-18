package fastgen

import (
	"SlamSim/matrixfactory"
	"testing"
)

func isLowerTube(v uint8) bool { return v == 23 || v == 24 || v == 27 || v == 28 }
func isUpperTube(v uint8) bool { return v == 25 || v == 26 || v == 29 || v == 30 }
func isRed(v uint8) bool       { return v >= 27 && v <= 30 }
func isHolder(v uint8) bool    { return v >= 17 && v <= 20 }

// checkRules проверяет расстановку по регламенту независимо от внутренностей генератора
func checkRules(t *testing.T, r Result, cfg Config) {
	t.Helper()
	m := r.Pattern
	at := func(c int) uint8 { return m[c/N][c%N] }

	ramps, sections, robots := 0, 0, 0
	var lower, upper, holders []int
	for c := 0; c < cellsN; c++ {
		v := at(c)
		switch {
		case v >= 3 && v <= 6:
			ramps++
		case v == 2 || v == 12:
			sections++
		case isUpperTube(v):
			sections++
			upper = append(upper, c)
		case isLowerTube(v):
			lower = append(lower, c)
		case isHolder(v):
			holders = append(holders, c)
		}
		if v == 11 || v == 12 {
			robots++
		}
	}
	if ramps != 10 || sections != 20 {
		t.Fatalf("ожидалось 10 пандусов и 20 секций, получено %d и %d\n%v", ramps, sections, m)
	}
	if robots != 1 || len(lower) != 2 || len(upper) != 1 || len(holders) != 3 {
		t.Fatalf("робот %d, трубы %d+%d, подставки %d\n%v", robots, len(lower), len(upper), len(holders), m)
	}

	// Цвета: 2+1
	tubes := append(append([]int{}, lower...), upper...)
	reds := 0
	for _, c := range tubes {
		if isRed(at(c)) {
			reds++
		}
	}
	if reds != 1 && reds != 2 {
		t.Fatalf("неверная комбинация цветов: %d красных", reds)
	}

	// Подставки: три соседних у одной границы, перед ними 2 свободных ряда
	id := at(holders[0])
	var depthDir int
	switch id {
	case 17:
		depthDir = N
	case 18:
		depthDir = -N
	case 19:
		depthDir = -1
	case 20:
		depthDir = 1
	}
	for _, h := range holders {
		if at(h) != id {
			t.Fatalf("подставки разных направлений\n%v", m)
		}
		for k := 1; k <= 2; k++ {
			v := at(h + depthDir*k)
			if v != 1 && v != 11 {
				t.Fatalf("перед подставкой %d занят перекрёсток (%d)\n%v", h, v, m)
			}
		}
	}
	if id == 17 || id == 18 {
		if holders[1] != holders[0]+1 || holders[2] != holders[1]+1 {
			t.Fatalf("подставки не в ряд %v", holders)
		}
	} else if holders[1] != holders[0]+N || holders[2] != holders[1]+N {
		t.Fatalf("подставки не в ряд %v", holders)
	}

	// Ни одна труба не стоит вплотную к концу пандуса
	for c := 0; c < cellsN; c++ {
		v := at(c)
		var ds []int
		if v == 3 || v == 4 {
			ds = []int{0, 1}
		} else if v == 5 || v == 6 {
			ds = []int{2, 3}
		}
		for _, d := range ds {
			u := neighbour[c][d]
			if u >= 0 && (isLowerTube(at(int(u))) || isUpperTube(at(int(u)))) {
				t.Fatalf("труба %d вплотную к пандусу %d\n%v", u, c, m)
			}
		}
	}
	if dist(lower[0], lower[1]) < cfg.MinLowerTubeDist {
		t.Fatalf("трубы нижнего яруса слишком близко")
	}

	checkRoute(t, r, tubes, holders)
}

// checkRoute проверяет маршрут по графу из matrixfactory (старая модель клеток)
func checkRoute(t *testing.T, r Result, tubes, holders []int) {
	t.Helper()
	fm, err := matrixfactory.NewFieldMatrix(r.Pattern)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := fm.ToGraph()
	if err != nil {
		t.Fatal(err)
	}
	edge := func(a, b int) bool {
		for _, e := range graph[uint16(a)] {
			if int(e.ToIDX) == b {
				return true
			}
		}
		return false
	}
	terminal := map[int]bool{}
	for _, c := range append(append([]int{}, tubes...), holders...) {
		terminal[c] = true
	}
	if len(r.Route) != 6 {
		t.Fatalf("ожидалось 6 сегментов, получено %d", len(r.Route))
	}
	visitedTubes := map[int]bool{}
	cost := 0
	total := 0
	for s, seg := range r.Route {
		total += len(seg)
		end := seg[len(seg)-1]
		v := r.Pattern[end/N][end%N]
		if s%2 == 0 && !(isLowerTube(v) || isUpperTube(v)) || s%2 == 1 && !isHolder(v) {
			t.Fatalf("сегмент %d заканчивается не там: %d", s, v)
		}
		if s%2 == 0 {
			if visitedTubes[end] {
				t.Fatalf("труба %d посещена дважды", end)
			}
			visitedTubes[end] = true
		}
		if s > 0 && seg[0] != r.Route[s-1][len(r.Route[s-1])-1] {
			t.Fatalf("сегмент %d не продолжает предыдущий", s)
		}
		for i := 0; i+1 < len(seg); i++ {
			if !edge(seg[i], seg[i+1]) {
				t.Fatalf("нет проезда %d→%d в графе matrixfactory\n%v\n%v", seg[i], seg[i+1], r.Pattern, r.Route)
			}
			if i > 0 && terminal[seg[i]] {
				t.Fatalf("маршрут проезжает сквозь трубу/подставку %d", seg[i])
			}
			if s > 0 {
				cost += cellWeight(r.Pattern[seg[i+1]/N][seg[i+1]%N])
			}
		}
		// выезд с трубы — туда же, откуда приехали
		if s%2 == 1 && seg[1] != r.Route[s-1][len(r.Route[s-1])-2] {
			t.Fatalf("робот уезжает с трубы не тем путём, которым подъехал")
		}
	}
	if total > routeMaxCells {
		t.Fatalf("маршрут длиннее формата файла: %d", total)
	}
	if cost != r.LogisticsCost {
		t.Fatalf("сумма секций %d не совпадает с LogisticsCost %d", cost, r.LogisticsCost)
	}

	// Труба другого цвета едет на среднюю подставку
	reds := 0
	for _, c := range tubes {
		if isRed(r.Pattern[c/N][c%N]) {
			reds++
		}
	}
	oddIsRed := reds == 1
	for s := 0; s < 6; s += 2 {
		tube := r.Route[s][len(r.Route[s])-1]
		holder := r.Route[s+1][len(r.Route[s+1])-1]
		isOdd := isRed(r.Pattern[tube/N][tube%N]) == oddIsRed
		if isOdd != (holder == holders[1]) {
			t.Fatalf("цвета не соответствуют доп. баллу")
		}
	}
}

func TestGeneratedFieldsFollowRules(t *testing.T) {
	cfg := DefaultConfig()
	g := New(1, 2, cfg)
	for i := 0; i < 3000; i++ {
		checkRules(t, g.Next(), cfg)
	}
}

func TestDeterministicBySeed(t *testing.T) {
	a := New(7, 7, DefaultConfig()).Next()
	b := New(7, 7, DefaultConfig()).Next()
	for y := range a.Pattern {
		for x := range a.Pattern[y] {
			if a.Pattern[y][x] != b.Pattern[y][x] {
				t.Fatal("одинаковый seed дал разные поля")
			}
		}
	}
}

func BenchmarkNext(b *testing.B) {
	g := New(1, 2, DefaultConfig())
	for b.Loop() {
		g.Next()
	}
}
