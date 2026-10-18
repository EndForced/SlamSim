package fastgen

const inf = 1 << 20

// paths хранит кратчайшие пути из нескольких клеток-источников.
// Вес ребра — вес клетки, в которую въезжаем, так что сумма по пути равна
// числу пройденных секций по регламенту (пандус — 3).
type paths struct {
	grid    *[cellsN]uint8
	blocked uint64
	srcSet  uint64
	dist    [cellsN][cellsN]int32
	parent  [cellsN][cellsN]int8
}

// reset готовит структуру к новой расстановке без новой аллокации
func (p *paths) reset(grid *[cellsN]uint8, blocked uint64) {
	p.grid, p.blocked, p.srcSet = grid, blocked, 0
}

// from считает Дейкстру из src (граф из 64 вершин — хватает O(V^2) без кучи)
func (p *paths) from(src int) {
	bit := uint64(1) << src
	if p.srcSet&bit != 0 {
		return
	}
	p.srcSet |= bit
	dist := &p.dist[src]
	parent := &p.parent[src]
	for i := range dist {
		dist[i] = inf
		parent[i] = -1
	}
	dist[src] = 0
	var done uint64
	for {
		v, best := -1, int32(inf)
		for i := 0; i < cellsN; i++ {
			if done&(1<<i) == 0 && dist[i] < best {
				v, best = i, dist[i]
			}
		}
		if v < 0 {
			return
		}
		done |= 1 << v
		for d := 0; d < 4; d++ {
			u := neighbour[v][d]
			if u < 0 || p.blocked&(1<<u) != 0 || !conn[p.grid[v]][p.grid[u]][d] {
				continue
			}
			nd := best + int32(cellWeight(p.grid[u]))
			if nd < dist[u] {
				dist[u] = nd
				parent[u] = int8(v)
			}
		}
	}
}

// path восстанавливает путь src→dst включительно (src уже посчитан через from)
func (p *paths) path(src, dst int) []int {
	var rev []int
	for v := dst; v != src; v = int(p.parent[src][v]) {
		rev = append(rev, v)
	}
	rev = append(rev, src)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

type plan struct {
	tubeOrder [3]int // порядок, в котором робот возит трубы
	holderOf  [3]int // holderOf[k] — индекс подставки (0..2) для трубы k
	approach  [3]int // клетка, с которой подъезжаем к трубе k
	logistics int
	firstLeg  int
}

var perms3 = [6][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}

// bestOrder перебирает порядок труб (6) и назначение подставок (6, с учётом цветов — 2)
// и для каждой трубы независимо выбирает лучший подъезд. Минимизируется сумма секций
// после первого касания трубы (п.8.3), при равенстве — путь от старта до первой трубы.
//
// Стоимость отрезка = 1 (выезд на соседний перекрёсток) + путь + 1 (въезд к трубе/подставке).
func bestOrder(sp *paths, robot int, approaches [3][]int, tr *holderTriplet, odd int, respectColors bool) (plan, bool) {
	best := plan{logistics: inf, firstLeg: inf}
	found := false
	for _, order := range perms3 {
		for _, hp := range perms3 {
			// hp[i] — подставка для i-й по счёту трубы
			if respectColors {
				ok := true
				for i := 0; i < 3; i++ {
					if (order[i] == odd) != (hp[i] == 1) {
						ok = false
					}
				}
				if !ok {
					continue
				}
			}
			var cur plan
			cur.tubeOrder = order
			valid := true
			for i := 0; i < 3 && valid; i++ {
				k := order[i]
				cur.holderOf[k] = hp[i]
				next := int(tr.inward[hp[i]])
				bestA, bestLog, bestFirst := -1, inf, inf
				for _, a := range approaches[k] {
					var log, first int
					after := int(sp.dist[a][next])
					if i == 0 {
						first = int(sp.dist[robot][a]) + 1
						log = 1 + after + 1
					} else {
						prev := int(tr.inward[hp[i-1]])
						log = 1 + int(sp.dist[prev][a]) + 1 + 1 + after + 1
					}
					if log < bestLog || log == bestLog && first < bestFirst {
						bestA, bestLog, bestFirst = a, log, first
					}
				}
				if bestA < 0 || bestLog >= inf {
					valid = false
					break
				}
				cur.approach[k] = bestA
				cur.logistics += bestLog
				if i == 0 {
					cur.firstLeg = bestFirst
				}
			}
			if !valid {
				continue
			}
			if cur.logistics < best.logistics || cur.logistics == best.logistics && cur.firstLeg < best.firstLeg {
				best = cur
				found = true
			}
		}
	}
	return best, found
}

// buildRoute собирает 6 сегментов маршрута в формате старого солвера
func (pl plan) buildRoute(sp *paths, robot int, tubes [3]int, tr *holderTriplet) [][]int {
	route := make([][]int, 0, 6)
	prev := robot
	prevEnd := -1 // клетка подставки, с которой выезжаем (или -1 для старта)
	for i := 0; i < 3; i++ {
		k := pl.tubeOrder[i]
		a := pl.approach[k]
		h := pl.holderOf[k]

		seg := make([]int, 0, 16)
		if prevEnd >= 0 {
			seg = append(seg, prevEnd)
		}
		seg = append(seg, sp.path(prev, a)...)
		seg = append(seg, tubes[k])
		route = append(route, seg)

		in := int(tr.inward[h])
		seg = make([]int, 0, 16)
		seg = append(seg, tubes[k])
		seg = append(seg, sp.path(a, in)...)
		seg = append(seg, int(tr.holders[h]))
		route = append(route, seg)

		prev = in
		prevEnd = int(tr.holders[h])
	}
	return route
}
