// Package fastgen — быстрый генератор расстановок «Ремонт труб» (РРО 2026, Будущие инженеры).
//
// Схема та же, что у жеребьёвки в регламенте: случайно расставить паттерны,
// поставить подставки, трубы и робота, затем проверить решаемость. Отличия от старого генератора:
//   - поле — массив [64]uint8 и битовые маски uint64 вместо мап и копий матриц;
//   - все установки паттернов, тройки подставок и таблица проездов посчитаны заранее;
//   - решаемость проверяется одним BFS, а маршрут ищется Дейкстрой из ≤16 точек
//     вместо 36 перестановок по 6 поисков пути;
//   - соблюдаются правила, которые старый генератор не проверял (см. Config и README).
package fastgen

import (
	"math"
	"math/rand/v2"
)

type Config struct {
	// Минимальное евклидово расстояние между двумя трубами нижнего яруса.
	// В регламенте его нет, это настройка разнообразия из старого генератора.
	MinLowerTubeDist float64
	// Маршрут-эталон кладёт трубу другого цвета на среднюю подставку
	// (иначе нельзя получить доп. балл за правильную установку).
	RespectColors bool
	// Сколько раз пробовать расставить трубы на одном поле, прежде чем строить новое
	TubeTries int
}

func DefaultConfig() Config {
	return Config{
		MinLowerTubeDist: 6,
		RespectColors:    true,
		TubeTries:        30,
	}
}

type Result struct {
	Pattern [][]uint8
	// Маршрут из 6 сегментов: робот→труба, труба→подставка, подставка→труба, ...
	// Соседние сегменты делят концевую клетку, как и в старом формате.
	Route [][]int
	// Эталонная сумма секций (п.3.2): от первого касания трубы до последней подставки,
	// трубы возятся по одной, пандус — 3 секции.
	LogisticsCost int
}

// Ограничения формата файла (file_operator): 112 клеток маршрута, длина сегмента в 6 битах
const routeMaxCells = 112
const segmentMaxLen = 63

type Generator struct {
	rng *rand.Rand
	cfg Config

	grid  [cellsN]uint8
	order []int
	cand  [][]int32 // буфер кандидатов для каждого уровня перебора
	steps int
	sp    paths
}

// New создаёт генератор. Каждой горутине нужен свой генератор.
func New(seed1, seed2 uint64, cfg Config) *Generator {
	g := &Generator{
		rng:   rand.New(rand.NewPCG(seed1, seed2)),
		cfg:   cfg,
		order: make([]int, len(placements)),
		cand:  make([][]int32, len(placements)),
	}
	for i := range g.cand {
		g.cand[i] = make([]int32, 0, 4*cellsN)
	}
	return g
}

// Next возвращает одну расстановку, удовлетворяющую регламенту, и эталонный маршрут
func (g *Generator) Next() Result {
	for {
		// Подставки выбираются первыми: зона из 9 перекрёстков резервируется до паттернов,
		// иначе ~97% случайных полей отбрасываются из-за отсутствия места под подставки
		tr := &triplets[g.rng.IntN(len(triplets))]
		if !g.placePatterns(tr.mask) {
			continue
		}
		if res, ok := g.fillField(tr); ok {
			return res
		}
	}
}

// placePatterns расставляет все 11 паттернов в случайных местах и направлениях
// перебором с возвратом, не занимая клетки из reserved
func (g *Generator) placePatterns(reserved uint64) bool {
	for i := range g.grid {
		g.grid[i] = idFloor
	}
	for i := range g.order {
		g.order[i] = i
	}
	g.rng.Shuffle(len(g.order), func(i, j int) { g.order[i], g.order[j] = g.order[j], g.order[i] })
	g.steps = 0
	occ := reserved
	return g.dfs(0, &occ)
}

const dfsBudget = 5000

func (g *Generator) dfs(depth int, occ *uint64) bool {
	if depth == len(g.order) {
		return true
	}
	pl := placements[g.order[depth]]
	c := g.cand[depth][:0]
	for i := range pl {
		if pl[i].mask&*occ == 0 {
			c = append(c, int32(i))
		}
	}
	for len(c) > 0 {
		g.steps++
		if g.steps > dfsBudget {
			return false
		}
		k := g.rng.IntN(len(c))
		p := &pl[c[k]]
		c[k] = c[len(c)-1]
		c = c[:len(c)-1]

		*occ |= p.mask
		if g.dfs(depth+1, occ) {
			for i := uint8(0); i < p.n; i++ {
				g.grid[p.idx[i]] = p.val[i]
			}
			return true
		}
		*occ &^= p.mask
	}
	return false
}

// fillField ставит подставки, робота и трубы на готовое поле с паттернами
func (g *Generator) fillField(tr *holderTriplet) (Result, bool) {
	grid := &g.grid

	// Перекрёстки на концах пандусов: регламент п.2.2 — между съездом пандуса и трубой
	// гарантируется перекрёсток, значит труба не может стоять вплотную к пандусу.
	var rampEnds uint64
	for v := 0; v < cellsN; v++ {
		id := grid[v]
		if id < 3 || id > 6 {
			continue
		}
		axis := [2]int{0, 1} // North, South
		if id == 5 || id == 6 {
			axis = [2]int{2, 3} // East, West
		}
		for _, d := range axis {
			if u := neighbour[v][d]; u >= 0 {
				rampEnds |= 1 << u
			}
		}
	}

	// 1. Подставки для установки труб: три у границы и два свободных ряда перед ними
	var holderMask uint64
	for _, h := range tr.holders {
		grid[h] = tr.id
		holderMask |= 1 << h
	}

	// 2. Связная область, из которой видны подставки (без труб)
	area := reach(grid, int(tr.inward[1]), holderMask)

	// 3. Робот — любой перекрёсток этой области (иначе подставки недостижимы)
	var robotCand []int
	for v := 0; v < cellsN; v++ {
		if area&(1<<v) != 0 && (grid[v] == idFloor || grid[v] == idCeil) {
			robotCand = append(robotCand, v)
		}
	}
	robot := robotCand[g.rng.IntN(len(robotCand))]

	// 4. Кандидаты для труб: две на нижнем ярусе, одна на секции верхнего (п.2.2, п.6)
	var lower, upper []int
	for v := 0; v < cellsN; v++ {
		bit := uint64(1) << v
		if area&bit == 0 || rampEnds&bit != 0 || v == robot {
			continue
		}
		switch grid[v] {
		case idFloor:
			if tr.front&bit == 0 {
				lower = append(lower, v)
			}
		case idCeil:
			upper = append(upper, v)
		}
	}
	if len(lower) < 2 || len(upper) == 0 {
		return Result{}, false
	}

	robotID := uint8(idRobotFloor)
	if grid[robot] == idCeil {
		robotID = idRobotCeil
	}
	grid[robot] = robotID

	for try := 0; try < g.cfg.TubeTries; try++ {
		i1 := g.rng.IntN(len(lower))
		i2 := g.rng.IntN(len(lower) - 1)
		if i2 >= i1 {
			i2++
		}
		tubes := [3]int{lower[i1], lower[i2], upper[g.rng.IntN(len(upper))]}
		if dist(tubes[0], tubes[1]) < g.cfg.MinLowerTubeDist {
			continue
		}
		res, ok := g.trySolve(tubes, tr, holderMask, robot)
		if ok {
			return res, true
		}
	}
	return Result{}, false
}

// trySolve проверяет, что при данных трубах задача решаема, и строит эталонный маршрут
func (g *Generator) trySolve(tubes [3]int, tr *holderTriplet, holderMask uint64, robot int) (Result, bool) {
	grid := g.grid // копия: трубы ставятся только в неё
	blocked := holderMask
	for k, t := range tubes {
		if k < 2 {
			grid[t] = idTubeFloor
		} else {
			grid[t] = idTubeCeil
		}
		blocked |= 1 << t
	}

	reachable := reach(&grid, robot, blocked)

	// Подъезды к трубам: достижимые соседи, из которых есть проезд к трубе.
	// Трубу потом кладём поперёк направления подъезда (п.6: подъезд с поперечной стороны).
	var approaches [3][]int
	for k, t := range tubes {
		for d := 0; d < 4; d++ {
			u := int(neighbour[t][d])
			if u < 0 || reachable&(1<<u) == 0 {
				continue
			}
			// d — направление от трубы к соседу, проезд нужен от соседа к трубе
			if conn[grid[u]][grid[t]][opposite(d)] {
				approaches[k] = append(approaches[k], u)
			}
		}
		if len(approaches[k]) == 0 {
			return Result{}, false
		}
	}
	for _, in := range tr.inward {
		if reachable&(1<<in) == 0 {
			return Result{}, false
		}
	}

	// Цвета: 2 красные + 1 синяя или 2 синие + 1 красная
	oddRed := g.rng.IntN(2) == 0
	odd := g.rng.IntN(3)
	var red [3]bool
	for k := range red {
		red[k] = (k == odd) == oddRed
	}

	// Дейкстра из каждой точки, где робот начинает отрезок пути
	sp := &g.sp
	sp.reset(&grid, blocked)
	sp.from(robot)
	for k := range tubes {
		for _, a := range approaches[k] {
			sp.from(a)
		}
	}
	for _, in := range tr.inward {
		sp.from(int(in))
	}

	best, ok := bestOrder(sp, robot, approaches, tr, odd, g.cfg.RespectColors)
	if !ok {
		return Result{}, false
	}

	route := best.buildRoute(sp, robot, tubes, tr)
	total := 0
	for _, seg := range route {
		if len(seg) > segmentMaxLen {
			return Result{}, false
		}
		total += len(seg)
	}
	if total > routeMaxCells {
		return Result{}, false
	}

	for k, t := range tubes {
		a := best.approach[k]
		grid[t] = coloredTube(k == 2, a/N != t/N, red[k])
	}

	pattern := make([][]uint8, N)
	for y := 0; y < N; y++ {
		pattern[y] = make([]uint8, N)
		copy(pattern[y], grid[y*N:(y+1)*N])
	}
	return Result{Pattern: pattern, Route: route, LogisticsCost: best.logistics}, true
}

// reach — BFS по проездам из start; клетки из blocked можно не проезжать насквозь и не посещать
func reach(grid *[cellsN]uint8, start int, blocked uint64) uint64 {
	var queue [cellsN]int8
	seen := uint64(1) << start
	queue[0] = int8(start)
	head, tail := 0, 1
	for head < tail {
		v := int(queue[head])
		head++
		for d := 0; d < 4; d++ {
			u := neighbour[v][d]
			if u < 0 {
				continue
			}
			bit := uint64(1) << u
			if seen&bit != 0 || blocked&bit != 0 || !conn[grid[v]][grid[u]][d] {
				continue
			}
			seen |= bit
			queue[tail] = u
			tail++
		}
	}
	return seen
}

func opposite(d int) int {
	return d ^ 1 // North<->South, East<->West
}

func dist(a, b int) float64 {
	ay, ax := a/N, a%N
	by, bx := b/N, b%N
	return math.Hypot(float64(ay-by), float64(ax-bx))
}
