package pathfind

import (
	"fmt"
	"math"
	"slices"
)

type GraphEdge struct {
	Weight             uint8
	ToIDX              uint16
	FromEndOrStartCell bool
}

// Это pathfinder все что он умеет - строить путь по взвешанному графу
// Он ничего не знает о клетках кроме того, являются они трубами (это осознанный комромисс)
type MatrixGraph map[uint16][]GraphEdge

type GraphProvider interface {
	ToGraph() (MatrixGraph, error)
}

type GraphNode struct {
	Edges            []GraphEdge
	Visited          bool
	CostFromStart    int
	IDX              int
	MustBeEndOrStart bool //to prevent tube penetration

}

func nodify(mg map[uint16][]GraphEdge) (map[int]GraphNode, error) {
	gn := make(map[int]GraphNode)
	for IDX, EdgeSlice := range mg {
		gn[int(IDX)] = GraphNode{
			Edges:            EdgeSlice,
			Visited:          false,
			CostFromStart:    math.MaxInt,
			IDX:              int(IDX),
			MustBeEndOrStart: EdgeSlice[0].FromEndOrStartCell,
		}
	}
	return gn, nil
}

var CachedGraph MatrixGraph

func NewMatrixGraph(gp GraphProvider) (MatrixGraph, error) {
	graph, err := gp.ToGraph()
	if err != nil {
		return MatrixGraph{}, fmt.Errorf("Error while creating MatrixGraph %v", err)
	}
	CachedGraph = graph
	return cloneGraph(CachedGraph), nil
}

func (m *MatrixGraph) UpdateGraph(gp GraphProvider) error {
	newGraph, err := gp.ToGraph()
	if err != nil {
		return fmt.Errorf("error while updating MatrixGraph: %w", err)
	}

	*m = newGraph
	return nil
}

func (m *MatrixGraph) PathFind(FromIDX int, ToIDX int) ([]int, error) {
	parentMap := make(map[int]int)
	nodeMap, err := nodify(*m)
	if err != nil {
		return []int{}, fmt.Errorf("Error while graph nodifying \n%v", err)
	}

	_, ok := nodeMap[FromIDX]
	_, ok1 := nodeMap[ToIDX]
	if !(ok && ok1) {
		return []int{}, fmt.Errorf("Unknown cell id while pathfinding\nFrom %v To%v", FromIDX, ToIDX)
	}

	startNode := nodeMap[FromIDX]
	startNode.CostFromStart = 0
	nodeMap[FromIDX] = startNode

	visited_IDX := make(map[int]bool)
	visited_IDX[FromIDX] = true

	for {
		cn, success := cheapestNode(nodeMap, ToIDX, FromIDX)
		if cn.IDX == ToIDX {
			return reconstructPath(ToIDX, FromIDX, parentMap), nil
		}
		if !success {
			return make([]int, 0), fmt.Errorf("Can't construct path\nFrom %v To %v", FromIDX, ToIDX)
		}

		cn.Visited = true
		nodeMap[cn.IDX] = cn
		nodeMap = updateConnections(nodeMap, cn, parentMap)

	}
}

func (m *MatrixGraph) CutCellsNeighboursExceptOne(targetCell int, allowedCell int) {
	for _, edge := range (*m)[uint16(targetCell)] {
		if edge.ToIDX == uint16(allowedCell) {
			(*m)[uint16((targetCell))] = []GraphEdge{edge}
			return
		}
	}
}

func (m *MatrixGraph) RestoreCellConnections() {
	*m = cloneGraph(CachedGraph)
}
func cloneGraph(src MatrixGraph) MatrixGraph {
	if src == nil {
		return nil
	}
	dst := make(MatrixGraph, len(src))
	for k, v := range src {
		// Копируем не только мапу, но и внутренние слайсы ребер
		edgesCopy := make([]GraphEdge, len(v))
		copy(edgesCopy, v)
		dst[k] = edgesCopy
	}
	return dst
}

func reconstructPath(endIDX int, startIDX int, pm map[int]int) []int {
	path := make([]int, 0, 20)

	currentIDX := endIDX
	path = append(path, currentIDX)

	if startIDX == endIDX {
		return path
	}

	for currentIDX != startIDX {
		parentIDX, exists := pm[currentIDX]

		if !exists {
			break
		}

		currentIDX = parentIDX
		path = append(path, currentIDX)
	}

	slices.Reverse(path)
	return path
}

func cheapestNode(gn map[int]GraphNode, wanted int, started int) (GraphNode, bool) {
	cheapestIdx := -1

	for idx, node := range gn {
		if node.Visited {
			continue
		}
		if node.CostFromStart == math.MaxInt {
			continue
		}
		if node.MustBeEndOrStart && node.IDX != wanted && node.IDX != started {
			continue
		}
		if cheapestIdx == -1 || node.CostFromStart < gn[cheapestIdx].CostFromStart {
			cheapestIdx = idx
		}
	}
	if cheapestIdx == -1 {
		return GraphNode{}, false
	}
	return gn[cheapestIdx], true
}

func updateConnections(gn map[int]GraphNode, node GraphNode, pm map[int]int) map[int]GraphNode {
	for _, edge := range node.Edges {
		curr_node := gn[int(edge.ToIDX)]

		if curr_node.Visited {
			continue
		}

		if node.CostFromStart+int(edge.Weight) < curr_node.CostFromStart {
			pm[curr_node.IDX] = node.IDX
			curr_node.CostFromStart = node.CostFromStart + int(edge.Weight)
		}
		gn[int(edge.ToIDX)] = curr_node
	}
	return gn
}
