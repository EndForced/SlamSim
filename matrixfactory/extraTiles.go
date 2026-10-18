package matrixfactory

const maxIDFromTiles = 22

//File to add tiles to Tiles from models.
//Because models is already kinda too big

var extraTiles = map[uint8]FieldPoint{
	maxIDFromTiles + 1: {
		Name:     "Синяя горизонтальная труба первого этажа",
		IDX:      maxIDFromTiles + 1,
		ConNorth: []ConnectionType{ConFloorTubeHorizon},
		ConSouth: []ConnectionType{ConFloorTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	maxIDFromTiles + 2: {
		Name:     "Синяя вертикальная труба первого этажа",
		IDX:      maxIDFromTiles + 2,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConFloorTubeVertical},
		ConWest:  []ConnectionType{ConFloorTubeVertical},
	},

	maxIDFromTiles + 3: {
		Name:     "Синяя горизонатльная труба второго этажа",
		IDX:      maxIDFromTiles + 3,
		ConNorth: []ConnectionType{ConCeilTubeHorizon},
		ConSouth: []ConnectionType{ConCeilTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	maxIDFromTiles + 4: {
		Name:     "Синяя вертикальная труба второго этажа",
		IDX:      maxIDFromTiles + 4,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConCeilTubeVertical},
		ConWest:  []ConnectionType{ConCeilTubeVertical},
	},

	maxIDFromTiles + 5: {
		Name:     "Красная горизонтальная труба первого этажа",
		IDX:      maxIDFromTiles + 5,
		ConNorth: []ConnectionType{ConFloorTubeHorizon},
		ConSouth: []ConnectionType{ConFloorTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	maxIDFromTiles + 6: {
		Name:     "Красная вертикальная труба первого этажа",
		IDX:      maxIDFromTiles + 6,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConFloorTubeVertical},
		ConWest:  []ConnectionType{ConFloorTubeVertical},
	},

	maxIDFromTiles + 7: {
		Name:     "Красная горизонатльная труба второго этажа",
		IDX:      maxIDFromTiles + 7,
		ConNorth: []ConnectionType{ConCeilTubeHorizon},
		ConSouth: []ConnectionType{ConCeilTubeHorizon},
		ConEast:  []ConnectionType{NoCon},
		ConWest:  []ConnectionType{NoCon},
	},

	maxIDFromTiles + 8: {
		Name:     "Красная вертикальная труба второго этажа",
		IDX:      maxIDFromTiles + 8,
		ConNorth: []ConnectionType{NoCon},
		ConSouth: []ConnectionType{NoCon},
		ConEast:  []ConnectionType{ConCeilTubeVertical},
		ConWest:  []ConnectionType{ConCeilTubeVertical},
	},
}

func init() {
	for key, val := range extraTiles {
		Tiles[key] = val
	}
}
