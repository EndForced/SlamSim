package pathfind

import (
	"fmt"
	"strings"
)

func (e GraphEdge) String() string {
	return fmt.Sprintf("{To: %d, W: %d, EOS: %v}", e.ToIDX, e.Weight, e.FromEndOrStartCell)
}

//vibe coding ngl

func (n GraphNode) String() string {
	// Собираем строковые представления всех ребер в компактный массив
	edgeStrings := make([]string, len(n.Edges))
	for i, edge := range n.Edges {
		edgeStrings[i] = edge.String()
	}
	edgesFormatted := "[" + strings.Join(edgeStrings, ", ") + "]"

	// Возвращаем аккуратную строку с именами полей
	return fmt.Sprintf("Node(IDX: %d, Visited: %t, Cost: %d, Edges: %s)",
		n.IDX, n.Visited, n.CostFromStart, edgesFormatted)
}
