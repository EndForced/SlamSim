// Описание поведения всех клеток и соединений между ними (полное)
package matrixfactory

import "fmt"

// hope you know what is north south east and west

// ConnectionType описывает тип стыка между двумя соседними клетками.
// Используется при построении графа: ребро между клетками появляется
// только если их типы соединений в соответствующем направлении совпадают.
type ConnectionType int

const (
	ConFloorFloor ConnectionType = iota
	ConCeilCeil

	ConFloorRampN
	ConFloorRampS
	ConFloorRampE
	ConFloorRampW

	ConCeilRampN
	ConCeilRampS
	ConCeilRampE
	ConCeilRampW

	ConFloorTubeHorizon
	ConFloorTubeVertical
	ConCeilTubeHorizon
	ConCeilTubeVertical
	// May be someday they will come handy
	// ConRampDownTube
	// ConRampUpTube

	ConBorder
	NoCon
)

// String возвращает человекочитаемое имя типа соединения.
// Для неизвестных значений возвращает "ConnectionType(<число>)".
func (c ConnectionType) String() string {
	switch c {
	case ConFloorFloor:
		return "ConFloorFloor"
	case ConCeilCeil:
		return "ConCeilCeil"
	case ConFloorRampN:
		return "ConFloorRampN"
	case ConFloorRampS:
		return "ConFloorRampS"
	case ConFloorRampE:
		return "ConFloorRampE"
	case ConFloorRampW:
		return "ConFloorRampW"
	case ConCeilRampN:
		return "ConCeilRampN"
	case ConCeilRampS:
		return "ConCeilRampS"
	case ConCeilRampE:
		return "ConCeilRampE"
	case ConCeilRampW:
		return "ConCeilRampW"
	case ConFloorTubeHorizon:
		return "ConFloorTubeHorizon"
	case ConFloorTubeVertical:
		return "ConFloorTubeVertical"
	case ConCeilTubeHorizon:
		return "ConCeilTubeHorizon"
	case ConCeilTubeVertical:
		return "ConCeilTubeVertical"
	case ConBorder:
		return "ConBorder"
	case NoCon:
		return "NoCon"
	default:
		return fmt.Sprintf("ConnectionType(%d)", c)
	}
}

//describing cell connection rules to build up the graph from cells ids

// weightGetter возвращает вес перехода для данного типа соединения.
// Рампы имеют вес 3, все остальные — 1.
func weightGetter(typ ConnectionType) int {
	//yea that is real programming
	switch typ {
	case ConFloorRampN:
		return 3
	case ConFloorRampS:
		return 3
	case ConFloorRampE:
		return 3
	case ConFloorRampW:
		return 3
	}
	return 1
}

// cellWeightGetter возвращает вес клетки по её ID.
// Рампы (ID 3–6) имеют вес 3, все остальные — 1.
func cellWeightGetter(typ uint8) int {
	//yea that is real programming one more time
	switch typ {
	case 3:
		return 3
	case 4:
		return 3
	case 5:
		return 3
	case 6:
		return 3
	}
	return 1
}

// TubeIDXGetter возвращает список ID клеток, являющихся трубами.
func (_ *FieldMatrix) TubeIDXGetter() []int {
	return []int{7, 8, 9, 10, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30}
}

// HolderIDXGetter возвращает список ID клеток, являющихся держателями труб.
func (_ *FieldMatrix) HolderIDXGetter() []int {
	return []int{17, 18, 19, 20}
}

// RobotIDXGetter возвращает список ID клеток, содержащих робота.
func (_ *FieldMatrix) RobotIDXGetter() []int {
	return []int{11, 12, 13, 14, 15, 16}
}

// GetHolderByDir возвращает ID держателя трубы, соответствующего
// заданному направлению (North → 17, South → 18, East → 19, West → 20).
// Для неизвестного направления возвращает 0.
func (_ *FieldMatrix) GetHolderByDir(dir DirectionType) int {
	if dir == North {
		return 17
	}
	if dir == South {
		return 18
	}
	if dir == East {
		return 19
	}
	if dir == West {
		return 20
	}
	return 0
}

// FieldPoint описывает тип клетки игрового поля: её имя, ID,
// наличие робота и возможные соединения по каждой из четырёх сторон.
type FieldPoint struct {
	Name     string
	IDX      uint8
	HasRobot bool
	ConNorth []ConnectionType
	ConSouth []ConnectionType
	ConEast  []ConnectionType
	ConWest  []ConnectionType
}

// Robotize_cell преобразует ID клетки: добавляет или убирает робота.
// При inverse == false ID 1–6 превращается в 11–16 (поставить робота).
// При inverse == true ID 11–16 превращается в 1–6 (убрать робота).
// Возвращает ошибку, если исходный ID не входит в допустимый диапазон.
// method to get rid of robot on cell
func Robotize_cell(inverse bool, id uint8) (uint8, error) {
	if !inverse {
		if id < 1 || id > 6 {
			return 0, fmt.Errorf("Unable to robotize cell %d", id)
		}
		return id + 10, nil
	}
	if inverse {
		if id < 11 || id > 16 {
			return 0, fmt.Errorf("Unable to derobotize cell %d", id)
		}
		return id - 10, nil
	}
	return 0, nil
}

// Tiles — глобальная карта всех типов клеток по их ID.
// Ключ — идентификатор клетки (0–22), значение — описание FieldPoint
// с именем и соединениями по каждой стороне.
var Tiles = map[uint8]FieldPoint{
	0: {
		Name:     "Пустота",
		IDX:      0,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	1: {
		Name: "Перекресток первый этаж",
		IDX:  1,
		ConNorth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampN,
			ConFloorTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampS,
			ConFloorTubeHorizon,
		},
		ConEast: []ConnectionType{
			ConFloorFloor,
			ConFloorRampE,
			ConFloorTubeVertical,
		},
		ConWest: []ConnectionType{
			ConFloorFloor,
			ConFloorRampW,
			ConFloorTubeVertical,
		},
	},

	2: {
		Name: "Перекресток второй этаж",
		IDX:  2,
		ConNorth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampN,
			ConCeilTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampS,
			ConCeilTubeHorizon,
		},
		ConEast: []ConnectionType{
			ConCeilCeil,
			ConCeilRampE,
			ConCeilTubeVertical,
		},
		ConWest: []ConnectionType{
			ConCeilCeil,
			ConCeilRampW,
			ConCeilTubeVertical,
		},
	},

	3: {
		Name: "Рампа заезжающая на север",
		IDX:  3,
		ConNorth: []ConnectionType{
			// ConCeilCeil,
			ConCeilRampN,
			ConCeilTubeHorizon,
		},
		ConSouth: []ConnectionType{
			// ConFloorFloor,
			ConFloorRampS,
			// ConFloorTubeHorizon,
		},
		ConEast: []ConnectionType{NoCon},
		ConWest: []ConnectionType{NoCon},
	},

	4: {
		Name: "Рампа заезжающая на юг",
		IDX:  4,
		ConNorth: []ConnectionType{
			// ConFloorFloor,
			ConFloorRampN,
			// ConFloorTubeHorizon,
		},
		ConSouth: []ConnectionType{
			// ConCeilCeil,
			ConCeilRampS,
			ConCeilTubeHorizon,
		},
		ConEast: []ConnectionType{NoCon},
		ConWest: []ConnectionType{NoCon},
	},

	5: {
		Name:     "Рампа заезжающая на восток",
		IDX:      5,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast: []ConnectionType{
			// ConCeilCeil,
			ConCeilRampW,
			ConCeilTubeVertical,
		},
		ConWest: []ConnectionType{
			// ConFloorFloor,
			ConFloorRampE,
			// ConFloorTubeVertical,
		},
	},

	6: {
		Name:     "Рампа заезжающая на запад",
		IDX:      6,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast: []ConnectionType{
			// ConFloorFloor,
			ConFloorRampE,
			// ConFloorTubeVertical,
		},
		ConWest: []ConnectionType{
			// ConCeilCeil,
			ConCeilRampW,
			ConCeilTubeVertical,
		},
	},

	7: {
		Name:     "Горизонтальная труба первого этажа",
		IDX:      7,
		ConNorth: []ConnectionType{ConFloorTubeHorizon},
		ConSouth: []ConnectionType{ConFloorTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	8: {
		Name:     "Вертикальная труба первого этажа",
		IDX:      8,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConFloorTubeVertical},
		ConWest:  []ConnectionType{ConFloorTubeVertical},
	},

	9: {
		Name:     "Горизонатльная труба второго этажа",
		IDX:      9,
		ConNorth: []ConnectionType{ConCeilTubeHorizon},
		ConSouth: []ConnectionType{ConCeilTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	10: {
		Name:     "Вертикальная труба второго этажа",
		IDX:      10,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConCeilTubeVertical},
		ConWest:  []ConnectionType{ConCeilTubeVertical},
	},

	11: {
		Name:     "Перекресток с роботом, первый этаж",
		IDX:      11,
		HasRobot: true,
		ConNorth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampN,
			ConFloorTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampS,
			ConFloorTubeHorizon,
		},
		ConEast: []ConnectionType{
			ConFloorFloor,
			ConFloorRampE,
			ConFloorTubeVertical,
		},
		ConWest: []ConnectionType{
			ConFloorFloor,
			ConFloorRampW,
			ConFloorTubeVertical,
		},
	},

	12: {
		Name:     "Перекресток с роботом, второй этаж",
		IDX:      12,
		HasRobot: true,
		ConNorth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampN,
			ConCeilTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampS,
			ConCeilTubeHorizon,
		},
		ConEast: []ConnectionType{
			ConCeilCeil,
			ConCeilRampE,
			ConCeilTubeVertical,
		},
		ConWest: []ConnectionType{
			ConCeilCeil,
			ConCeilRampW,
			ConCeilTubeVertical,
		},
	},

	13: {
		Name:     "Рампа с робтом, заезжающая на север",
		IDX:      13,
		HasRobot: true,
		ConNorth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampN,
			ConCeilTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampS,
			ConFloorTubeHorizon,
		},
		ConEast: []ConnectionType{NoCon},
		ConWest: []ConnectionType{NoCon},
	},

	14: {
		Name:     "Рампа с роботом заезжающая на юг",
		IDX:      14,
		HasRobot: true,
		ConNorth: []ConnectionType{
			ConFloorFloor,
			ConFloorRampN,
			ConFloorTubeHorizon,
		},
		ConSouth: []ConnectionType{
			ConCeilCeil,
			ConCeilRampS,
			ConCeilTubeHorizon,
		},
		ConEast: []ConnectionType{NoCon},
		ConWest: []ConnectionType{NoCon},
	},

	15: {
		Name:     "Рампа с роботом, заезжающая на восток",
		IDX:      15,
		HasRobot: true,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast: []ConnectionType{
			ConCeilCeil,
			ConCeilRampE,
			ConCeilTubeVertical,
		},
		ConWest: []ConnectionType{
			ConFloorFloor,
			ConFloorRampW,
			ConFloorTubeVertical,
		},
	},

	16: {
		Name:     "Рампа с роботом, заезжающая на запад",
		IDX:      16,
		HasRobot: true,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast: []ConnectionType{
			ConFloorFloor,
			ConFloorRampE,
			ConFloorTubeVertical,
		},
		ConWest: []ConnectionType{
			ConCeilCeil,
			ConCeilRampW,
			ConCeilTubeVertical,
		},
	},

	17: {
		Name:     "Держатель трубы северный",
		IDX:      17,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{ConFloorTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	18: {
		Name:     "Держатель трубы южный",
		IDX:      18,
		ConNorth: []ConnectionType{ConFloorTubeHorizon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	19: {
		Name:     "Держатель трубы восточный",
		IDX:      19,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{ConFloorTubeVertical},
	},

	20: {
		Name:     "Держатель трубы западный",
		IDX:      20,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConFloorTubeVertical},
		ConWest:  []ConnectionType{NoCon},
	},

	21: {
		Name:     "Универсальная труба первого этажа", // Нужна для генерации расстановок
		IDX:      21,
		ConNorth: []ConnectionType{ConFloorTubeVertical, ConFloorTubeHorizon},
		ConSouth: []ConnectionType{ConFloorTubeVertical, ConFloorTubeHorizon},
		ConEast:  []ConnectionType{ConFloorTubeVertical, ConFloorTubeHorizon},
		ConWest:  []ConnectionType{ConFloorTubeVertical, ConFloorTubeHorizon},
	},

	22: {
		Name:     "Универсальная труба второго этажа", // Нужна для генерации расстановок
		IDX:      22,
		ConNorth: []ConnectionType{ConCeilTubeVertical, ConCeilTubeHorizon},
		ConSouth: []ConnectionType{ConCeilTubeVertical, ConCeilTubeHorizon},
		ConEast:  []ConnectionType{ConCeilTubeVertical, ConCeilTubeHorizon},
		ConWest:  []ConnectionType{ConCeilTubeVertical, ConCeilTubeHorizon},
	},
}
