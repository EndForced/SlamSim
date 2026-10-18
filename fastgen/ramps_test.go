package fastgen

import (
	"SlamSim/matrixfactory"
	"testing"
)

// Каждый пандус проезжается только вдоль подъёма: снизу — с пола, сверху — с секции,
// в обе стороны. Сбоку, с секции снизу и с пола сверху въезда нет.
func TestRampsAllDirections(t *testing.T) {
	ramps := map[uint8]int{3: int(matrixfactory.North), 4: int(matrixfactory.South), 5: int(matrixfactory.East), 6: int(matrixfactory.West)}
	for id, up := range ramps {
		for _, r := range []uint8{id, id + 10} { // сам пандус и пандус с роботом
			for d := 0; d < 4; d++ {
				dir := matrixfactory.DirectionType(d)
				back := matrixfactory.DirectionType(d ^ 1)
				fromFloor := Connects(1, r, dir) // пол стоит с противоположной от d стороны пандуса
				fromCeil := Connects(2, r, dir)
				toFloor := Connects(r, 1, back)
				toCeil := Connects(r, 2, back)
				wantFloor := d == up  // к низу пандуса подъезжаем по направлению подъёма
				wantCeil := d == up^1 // к верху — по направлению спуска
				if fromFloor != wantFloor || toFloor != wantFloor {
					t.Errorf("тайл %d, движение %d: пол→пандус %v, пандус→пол %v, ожидалось %v", r, d, fromFloor, toFloor, wantFloor)
				}
				if fromCeil != wantCeil || toCeil != wantCeil {
					t.Errorf("тайл %d, движение %d: секция→пандус %v, пандус→секция %v, ожидалось %v", r, d, fromCeil, toCeil, wantCeil)
				}
			}
		}
	}
	// Пол и секция напрямую не соединены
	for d := 0; d < 4; d++ {
		if Connects(1, 2, matrixfactory.DirectionType(d)) || Connects(2, 1, matrixfactory.DirectionType(d)) {
			t.Errorf("пол и секция соединены напрямую в направлении %d", d)
		}
	}
}

// Пандусы соединены между собой только верхними концами вдоль оси: «горка» из паттерна {5,6}
// проезжается насквозь (так было и в исходной модели matrixfactory). Других соединений нет.
func TestRampToRampOnlyHill(t *testing.T) {
	up := map[uint8]int{3: int(matrixfactory.North), 4: int(matrixfactory.South), 5: int(matrixfactory.East), 6: int(matrixfactory.West)}
	for a := uint8(3); a <= 6; a++ {
		for b := uint8(3); b <= 6; b++ {
			for d := 0; d < 4; d++ {
				// горка: a поднимается в направлении d, b — навстречу
				want := up[a] == d && up[b] == d^1
				if got := Connects(a, b, matrixfactory.DirectionType(d)); got != want {
					t.Errorf("пандус %d → пандус %d в направлении %d: %v, ожидалось %v", a, b, d, got, want)
				}
			}
		}
	}
}
