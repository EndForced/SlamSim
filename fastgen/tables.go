package fastgen

import "SlamSim/matrixfactory"

// Предвычисленные таблицы: всё, что не зависит от конкретной расстановки,
// считается один раз при старте программы, чтобы генерация шла без аллокаций и мап.

const N = 8          // размер поля (формат файла жёстко 8x8)
const cellsN = N * N // количество перекрёстков

// Направления в том же порядке, что и в matrixfactory: North, South, East, West
var dy = [4]int{-1, 1, 0, 0}
var dx = [4]int{0, 0, 1, -1}

// ID клеток из matrixfactory
const (
	idFloor      = 1
	idCeil       = 2
	idRobotFloor = 11
	idRobotCeil  = 12
	idTubeFloor  = 21 // универсальная труба нижнего яруса (до выбора ориентации)
	idTubeCeil   = 22 // универсальная труба верхнего яруса
	maxID        = 31
)

// Набор паттернов из регламента (рис. 5): 10 пандусов и 20 секций.
// Записаны в базовом направлении East (вставка слева направо).
var basePatterns = [][]uint8{
	{2}, {2}, {2},
	{2, 2}, {5, 6},
	{5, 2, 2}, {5, 2, 2}, {5, 2, 2, 2}, {5, 2, 2, 2},
	{5, 2, 2, 6}, {5, 2, 2, 2, 6},
}

// neighbour[v][d] — индекс соседней клетки или -1 за границей поля
var neighbour [cellsN][4]int8

// conn[a][b][d] — есть ли проезд из клетки типа a в соседнюю клетку типа b в направлении d.
// Строится из matrixfactory.Tiles по тем же правилам, что и FieldMatrix.ToGraph,
// поэтому модель клеток остаётся единственным источником правды.
var conn [maxID][maxID][4]bool

// placement — один вариант установки паттерна: битовая маска занятых клеток и значения
type placement struct {
	mask uint64
	n    uint8
	idx  [5]uint8
	val  [5]uint8
}

// placements[p] — все допустимые (в границах поля) установки паттерна p во всех 4 направлениях
var placements [][]placement

// holderTriplet — три подставки в ряд вдоль границы поля и два свободных ряда перед ними
type holderTriplet struct {
	holders [3]uint8 // по порядку вдоль границы, holders[1] — средняя
	inward  [3]uint8 // перекрёсток прямо перед каждой подставкой
	id      uint8    // ID держателя (17-20) в зависимости от границы
	mask    uint64   // подставки + 6 перекрёстков перед ними, всё должно быть пустым полом
	front   uint64   // только 6 перекрёстков перед подставками
}

var triplets []holderTriplet

func init() {
	for v := 0; v < cellsN; v++ {
		y, x := v/N, v%N
		for d := 0; d < 4; d++ {
			ny, nx := y+dy[d], x+dx[d]
			if ny < 0 || ny >= N || nx < 0 || nx >= N {
				neighbour[v][d] = -1
			} else {
				neighbour[v][d] = int8(ny*N + nx)
			}
		}
	}

	for a := 0; a < maxID; a++ {
		ta, okA := matrixfactory.Tiles[uint8(a)]
		if !okA {
			continue
		}
		for b := 0; b < maxID; b++ {
			tb, okB := matrixfactory.Tiles[uint8(b)]
			if !okB {
				continue
			}
			conn[a][b][matrixfactory.North] = connects(ta.ConNorth, tb.ConSouth)
			conn[a][b][matrixfactory.South] = connects(ta.ConSouth, tb.ConNorth)
			conn[a][b][matrixfactory.East] = connects(ta.ConEast, tb.ConWest)
			conn[a][b][matrixfactory.West] = connects(ta.ConWest, tb.ConEast)
		}
	}

	for _, pat := range basePatterns {
		placements = append(placements, buildPlacements(pat))
	}

	buildTriplets()
}

// connects повторяет логику intersect из matrixfactory: стык есть, если у сторон
// совпадает хотя бы один тип соединения, отличный от NoCon
func connects(a, b []matrixfactory.ConnectionType) bool {
	for _, x := range a {
		if x == matrixfactory.NoCon {
			continue
		}
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// rotate возвращает паттерн в направлении dir и признак горизонтальной вставки.
// Повороты те же, что и в matrixgenerator.rotatePattern2Cache.
func rotate(dir matrixfactory.DirectionType, pat []uint8) ([]uint8, bool) {
	out := make([]uint8, len(pat))
	copy(out, pat)
	swap := map[uint8]uint8{}
	reverse := false
	horizontal := false
	switch dir {
	case matrixfactory.East:
		horizontal = true
	case matrixfactory.West:
		swap[5], swap[6] = 6, 5
		reverse, horizontal = true, true
	case matrixfactory.North:
		swap[5], swap[6] = 4, 3
	case matrixfactory.South:
		swap[5], swap[6] = 3, 4
		reverse = true
	}
	for i, v := range out {
		if s, ok := swap[v]; ok {
			out[i] = s
		}
	}
	if reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, horizontal
}

func buildPlacements(pat []uint8) []placement {
	var res []placement
	dirs := []matrixfactory.DirectionType{matrixfactory.North, matrixfactory.South, matrixfactory.East, matrixfactory.West}
	for _, dir := range dirs {
		rot, horizontal := rotate(dir, pat)
		l := len(rot)
		for y := 0; y < N; y++ {
			for x := 0; x < N; x++ {
				if horizontal && x+l > N || !horizontal && y+l > N {
					continue
				}
				p := placement{n: uint8(l)}
				for i := 0; i < l; i++ {
					cy, cx := y, x+i
					if !horizontal {
						cy, cx = y+i, x
					}
					c := cy*N + cx
					p.idx[i] = uint8(c)
					p.val[i] = rot[i]
					p.mask |= 1 << c
				}
				res = append(res, p)
			}
		}
	}
	return res
}

// buildTriplets перебирает все тройки соседних перекрёстков вдоль каждой границы.
// Регламент п.2.3: подставки ставятся в ряд на соседних линиях, примыкая к границе поля,
// перед ними должны быть свободны два ряда перекрёстков (всего шесть).
func buildTriplets() {
	type edge struct {
		id         uint8
		cell       func(i, depth int) int // i — позиция вдоль границы, depth — ряд от границы
		horizontal bool
	}
	edges := []edge{
		{17, func(i, d int) int { return d*N + i }, true},            // верхняя граница, держатель северный
		{18, func(i, d int) int { return (N-1-d)*N + i }, true},      // нижняя граница, держатель южный
		{19, func(i, d int) int { return i*N + (N - 1 - d) }, false}, // правая граница, держатель восточный
		{20, func(i, d int) int { return i*N + d }, false},           // левая граница, держатель западный
	}
	for _, e := range edges {
		for s := 0; s+3 <= N; s++ {
			t := holderTriplet{id: e.id}
			for k := 0; k < 3; k++ {
				h := e.cell(s+k, 0)
				t.holders[k] = uint8(h)
				t.inward[k] = uint8(e.cell(s+k, 1))
				t.mask |= 1 << h
				for depth := 1; depth <= 2; depth++ {
					c := e.cell(s+k, depth)
					t.mask |= 1 << c
					t.front |= 1 << c
				}
			}
			triplets = append(triplets, t)
		}
	}
}

func cellWeight(id uint8) int {
	if id >= 3 && id <= 6 {
		return 3 // проезд по пандусу считается за 3 секции (п.3.2)
	}
	return 1
}

// coloredTube возвращает ID цветной трубы (matrixfactory extraTiles) по ярусу,
// направлению подъезда и цвету. Труба кладётся поперёк направления подъезда.
func coloredTube(upper bool, approachVertical bool, red bool) uint8 {
	var base uint8 = 23 // синяя горизонтальная первого этажа
	if upper {
		base += 2
	}
	if !approachVertical {
		base++
	}
	if red {
		base += 4
	}
	return base
}

// Connects сообщает, есть ли проезд из клетки типа a в соседнюю клетку типа b в направлении dir
// (matrixfactory.North/South/East/West). Используется симулятором.
func Connects(a, b uint8, dir matrixfactory.DirectionType) bool {
	if int(a) >= maxID || int(b) >= maxID {
		return false
	}
	return conn[a][b][dir]
}

// CellWeight — сколько секций засчитывается за въезд в клетку (пандус — 3)
func CellWeight(id uint8) int {
	return cellWeight(id)
}
