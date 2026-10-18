package matrixfactory

import (
	"SlamSim/pathfind"
	"fmt"
)

// клетка поля idx (y * n + x) n - размер матрицы
type fieldTile struct {
	IDX      int16 // max - n ^ 2 - 1
	TileType FieldPoint
}

type FieldMatrix struct {
	size      uint8
	hasRobot  bool
	robotPos  [2]uint8
	tiles     map[int16]fieldTile
	tiles_raw [][]uint8
}

type DirectionType int

const (
	North DirectionType = iota
	South
	East
	West
)

type neighbours struct {
	North fieldTile
	South fieldTile
	East  fieldTile
	West  fieldTile
}

func (m *FieldMatrix) MoveRobot(desty uint8, destx uint8) error {
	if m.size <= desty || m.size <= destx {
		return fmt.Errorf("Unable to move robot to (%d, %d) with matrix size %d", destx, desty, m.size)
	}

	y, x := m.robotPos[0], m.robotPos[1]
	if desty == y && destx == x {
		return nil
	}

	if m.hasRobot {
		derobotized, err := Robotize_cell(true, m.tiles_raw[y][x])
		if err != nil {
			return err
		}

		m.tiles_raw[y][x] = derobotized
		m.tiles[int16(int(y)*int(m.size)+int(x))] =
			fieldTile{
				IDX:      int16(y*m.size + x),
				TileType: Tiles[derobotized],
			}
	}

	robotized, err := Robotize_cell(false, m.tiles_raw[desty][destx])
	if err != nil {
		return err
	}

	m.tiles_raw[desty][destx] = robotized
	m.tiles[int16(desty*m.size+destx)] = fieldTile{IDX: int16(desty*m.size + destx), TileType: Tiles[robotized]}

	m.robotPos = [2]uint8{desty, destx}
	m.hasRobot = true
	return nil
}

func NewFieldMatrix(matrix [][]uint8) (*FieldMatrix, error) {
	if len(matrix) == 0 {
		return &FieldMatrix{}, fmt.Errorf("You passed an empty matrix to its constructor")
	}

	rCount := 0
	msize := len(matrix)
	tmap := make(map[int16]fieldTile)
	var rPos [2]uint8

	for y := 0; y < msize; y++ {
		for x := 0; x < msize; x++ {
			tmap[int16(y*msize+x)] = fieldTile{
				IDX:      int16(y*msize + x),
				TileType: Tiles[matrix[y][x]],
			}
			if Tiles[matrix[y][x]].HasRobot {
				rCount++
				rPos = [2]uint8{uint8(y), uint8(x)}
			}
		}

	}
	if rCount > 1 {
		return &FieldMatrix{}, fmt.Errorf("Can't construct matrix with more than 1 robot")
	}

	return &FieldMatrix{
		size:      uint8(msize),
		hasRobot:  rCount > 0,
		robotPos:  rPos,
		tiles:     tmap,
		tiles_raw: matrix,
	}, nil
}

func (m *FieldMatrix) RawData() [][]uint8 {
	if m.tiles_raw == nil {
		return nil
	}

	// Создаем новый срез для строк
	copiedRaw := make([][]uint8, len(m.tiles_raw))
	for i := range m.tiles_raw {
		// Создаем новый срез для каждого столбца и копируем данные
		copiedRaw[i] = make([]uint8, len(m.tiles_raw[i]))
		copy(copiedRaw[i], m.tiles_raw[i])
	}

	return copiedRaw
}

func (m *FieldMatrix) GetTiles() map[int16]fieldTile {
	copiedTiles := make(map[int16]fieldTile, len(m.tiles))
	for k, v := range m.tiles {
		copiedTiles[k] = v
	}
	return copiedTiles
}

func (m *FieldMatrix) ToGraph() (pathfind.MatrixGraph, error) {
	gm := make(pathfind.MatrixGraph)
	s := int(m.size)

	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			currIDX := uint16(y*s + x)
			// fmt.Printf("%v", currIDX)
			currCell := m.tiles[int16(currIDX)]
			neigh, err := m.getNeighbours(currIDX)
			if err != nil {
				return nil, fmt.Errorf("error while graph constructing: %v", err)
			}
			nodes := m.handleNeighbours(currCell.TileType, neigh)
			// fmt.Printf("IDX %v Nodes %v", currIDX, nodes)
			if len(nodes) > 0 {
				gm[currIDX] = nodes
			}
		}
	}

	return gm, nil
}

func (m *FieldMatrix) handleNeighbours(cell FieldPoint, neigh neighbours) []pathfind.GraphEdge {
	res := make([]pathfind.GraphEdge, 0, 4)
	var endOrStart bool = false

	endOrStartCells := m.TubeIDXGetter()
	endOrStartMap := make(map[int]struct{})
	for _, idx := range endOrStartCells {
		endOrStartMap[idx] = struct{}{}
	}

	_, ok := endOrStartMap[int(cell.IDX)]
	if ok {
		endOrStart = true
	}

	// fmt.Printf("%v", neigh)
	// 1. СЕВЕР (North -> South)

	if interType, ok := intersect(cell.ConNorth, neigh.North.TileType.ConSouth); ok && interType != NoCon {
		// fmt.Println("North")
		res = append(res, pathfind.GraphEdge{
			Weight:             uint8(weightGetter(interType)),
			ToIDX:              uint16(neigh.North.IDX),
			FromEndOrStartCell: endOrStart,
		})
	}

	// 2. ЮГ (South -> North)
	if interType, ok := intersect(cell.ConSouth, neigh.South.TileType.ConNorth); ok && interType != NoCon {
		// fmt.Println("South")
		res = append(res, pathfind.GraphEdge{
			Weight:             uint8(weightGetter(interType)),
			ToIDX:              uint16(neigh.South.IDX),
			FromEndOrStartCell: endOrStart,
		})
	}

	// 3. ВОСТОК (East -> West)
	if interType, ok := intersect(cell.ConEast, neigh.East.TileType.ConWest); ok && interType != NoCon {
		// fmt.Println("West")
		res = append(res, pathfind.GraphEdge{
			Weight:             uint8(weightGetter(interType)),
			ToIDX:              uint16(neigh.East.IDX),
			FromEndOrStartCell: endOrStart,
		})
	}

	// 4. ЗАПАД (West -> East)
	if interType, ok := intersect(cell.ConWest, neigh.West.TileType.ConEast); ok && interType != NoCon {
		// fmt.Println("East")
		res = append(res, pathfind.GraphEdge{
			Weight:             uint8(weightGetter(interType)),
			ToIDX:              uint16(neigh.West.IDX),
			FromEndOrStartCell: endOrStart,
		})
	}
	return res
}

func intersect(slice1, slice2 []ConnectionType) (ConnectionType, bool) {
	if len(slice1) == 0 || len(slice2) == 0 {
		return NoCon, false
	}

	if len(slice1) > len(slice2) {
		slice1, slice2 = slice2, slice1
	}

	set := make(map[ConnectionType]struct{}, len(slice1))
	for _, val := range slice1 {
		set[val] = struct{}{}
	}

	for _, val := range slice2 {
		if _, exists := set[val]; exists {
			// fmt.Printf("Type: %v", val)
			return val, true
		}
	}

	return NoCon, false
}

func (matrix_big *FieldMatrix) getNeighbours(idx uint16) (neighbours, error) {
	// indexes := make([]int32, 4)
	//that function sucks - ngl, but i need proper orientation sooo bad
	matrix := matrix_big.tiles_raw
	size := len(matrix)
	res := neighbours{}

	if int(idx) >= (size * size) {
		return res, fmt.Errorf("Error while getting neighbours for %d cell, matrix is too small", idx)
	}

	dx := [4]int{0, 0, 1, -1}
	dy := [4]int{-1, 1, 0, 0}

	for i := 0; i < 4; i++ {
		cellx, celly := int(idx)%size, int(idx)/size
		cellx = cellx + dx[i]
		celly = celly + dy[i]

		if !((0 <= celly) && (size > celly) && (0 <= cellx) && (size > cellx)) {
			switch i {
			case 0:
				res.North = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[0]}

			case 1:
				res.South = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[0]}

			case 2:
				res.East = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[0]}

			case 3:
				res.West = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[0]}

			}
			continue
		}

		numeric := matrix[celly][cellx]
		switch i {
		case 0:
			res.North = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[numeric]}

		case 1:
			res.South = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[numeric]}

		case 2:
			res.East = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[numeric]}

		case 3:
			res.West = fieldTile{IDX: int16(celly*size + cellx), TileType: Tiles[numeric]}
		}
	}
	return res, nil
}
func (mat *FieldMatrix) CalculateRouteLenght(route []int) int {
	var fullSum int
	for _, idx := range route {
		y := idx / int(mat.size)
		x := idx % int(mat.size)
		cell_idx := int(mat.RawData()[y][x])
		// fmt.Printf("\n%v Y: %v X: %v", cell_idx, y, x)
		fullSum += cellWeightGetter(uint8(cell_idx))
	}
	return fullSum
}
