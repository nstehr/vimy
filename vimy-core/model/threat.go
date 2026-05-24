package model

// ThreatField is a zone-parallel array to TerrainGrid.Grid. Each cell holds
// a cumulative threat weight produced by enemy defenses; higher = more
// dangerous to traverse. Values are unbounded but callers should treat them
// as relative, not absolute.
type ThreatField struct {
	Cols, Rows int
	Cells      []float64
}

// NewThreatField allocates a zero-initialized field matching the grid shape.
func NewThreatField(g *TerrainGrid) *ThreatField {
	if g == nil {
		return nil
	}
	return &ThreatField{Cols: g.Cols, Rows: g.Rows, Cells: make([]float64, g.Cols*g.Rows)}
}

// At returns the threat weight at (col,row), or 0 out of bounds.
func (f *ThreatField) At(col, row int) float64 {
	if col < 0 || col >= f.Cols || row < 0 || row >= f.Rows {
		return 0
	}
	return f.Cells[row*f.Cols+col]
}

// AddSource paints a defense's threat onto the field centered on the zone
// containing (mapX, mapY). The source cell gets the full weight; 4-neighbors
// get weight/2; diagonals get weight/3. Cumulative across calls so overlapping
// defenses create hotter zones.
func (f *ThreatField) AddSource(g *TerrainGrid, mapX, mapY int, weight float64) {
	if f == nil || g == nil || g.CellW <= 0 || g.CellH <= 0 {
		return
	}
	col := mapX / g.CellW
	row := mapY / g.CellH
	f.add(col, row, weight)
	f.add(col-1, row, weight/2)
	f.add(col+1, row, weight/2)
	f.add(col, row-1, weight/2)
	f.add(col, row+1, weight/2)
	f.add(col-1, row-1, weight/3)
	f.add(col+1, row-1, weight/3)
	f.add(col-1, row+1, weight/3)
	f.add(col+1, row+1, weight/3)
}

func (f *ThreatField) add(col, row int, w float64) {
	if col < 0 || col >= f.Cols || row < 0 || row >= f.Rows {
		return
	}
	f.Cells[row*f.Cols+col] += w
}

// ApproachPath returns the weighted-shortest passable zone path from `from`
// to `to`, where each zone's traversal cost is `1 + threat * penalty`.
// Returns nil if `to` is unreachable or inputs are invalid.
//
// A higher penalty biases the path to skirt threat zones; penalty=0 reduces
// to plain BFS shortest path.
func ApproachPath(g *TerrainGrid, threat *ThreatField, from, to [2]int, penalty float64) [][2]int {
	if g == nil {
		return nil
	}
	if !approachInBounds(g, from[0], from[1]) || !approachInBounds(g, to[0], to[1]) {
		return nil
	}
	if !passable(g.At(from[0], from[1])) || !passable(g.At(to[0], to[1])) {
		return nil
	}

	n := g.Cols * g.Rows
	dist := make([]float64, n)
	parent := make([]int, n)
	visited := make([]bool, n)
	for i := range dist {
		dist[i] = -1
		parent[i] = -1
	}

	srcIdx := from[1]*g.Cols + from[0]
	dstIdx := to[1]*g.Cols + to[0]
	dist[srcIdx] = 0

	// Dijkstra with a simple scan — 1024 cells, good enough without a heap.
	for {
		cur := -1
		bestD := -1.0
		for i := 0; i < n; i++ {
			if visited[i] || dist[i] < 0 {
				continue
			}
			if cur == -1 || dist[i] < bestD {
				cur = i
				bestD = dist[i]
			}
		}
		if cur == -1 || cur == dstIdx {
			break
		}
		visited[cur] = true

		col := cur % g.Cols
		row := cur / g.Cols
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nc, nr := col+d[0], row+d[1]
			if !approachInBounds(g, nc, nr) || !passable(g.At(nc, nr)) {
				continue
			}
			nidx := nr*g.Cols + nc
			if visited[nidx] {
				continue
			}
			cost := 1.0
			if threat != nil {
				cost += threat.At(nc, nr) * penalty
			}
			nd := dist[cur] + cost
			if dist[nidx] < 0 || nd < dist[nidx] {
				dist[nidx] = nd
				parent[nidx] = cur
			}
		}
	}

	if dist[dstIdx] < 0 {
		return nil
	}
	var rev [][2]int
	for cur := dstIdx; cur != -1; cur = parent[cur] {
		rev = append(rev, [2]int{cur % g.Cols, cur / g.Cols})
		if cur == srcIdx {
			break
		}
	}
	path := make([][2]int, len(rev))
	for i := range rev {
		path[i] = rev[len(rev)-1-i]
	}
	return path
}

func approachInBounds(g *TerrainGrid, col, row int) bool {
	return col >= 0 && col < g.Cols && row >= 0 && row < g.Rows
}
