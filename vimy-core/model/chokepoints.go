package model

// ChokepointKind classifies why a zone is a chokepoint.
type ChokepointKind byte

const (
	ChokeBridge     ChokepointKind = 0 // Bridge zone — always a choke
	ChokeLandNarrow ChokepointKind = 1 // Land zone squeezed between impassable neighbors
)

// Chokepoint is a grid zone that constricts movement. Score is a relative
// ranking, not a probability.
type Chokepoint struct {
	Col, Row int
	Kind     ChokepointKind
	Score    float64
}

const (
	bridgeScore        = 0.9 // bridges always outrank land strips
	narrowDeepScore    = 0.7 // land strip with 2-cell-deep walls on both sides
	narrowShallowScore = 0.5 // land strip with 1-cell-deep walls on both sides

	pathBoost     = 0.1  // bonus when choke lies on BFS shortest path
	pathNearBoost = 0.05 // smaller bonus when within 1 zone of the path
)

// passable includes Bridge — which is the whole point of bridges being chokes.
func passable(t TerrainType) bool {
	return t == Land || t == Bridge
}

// FindChokepoints returns bridges first, in row-major order, then narrow land
// strips.
func FindChokepoints(g *TerrainGrid) []Chokepoint {
	if g == nil || g.Cols <= 0 || g.Rows <= 0 {
		return nil
	}

	var out []Chokepoint

	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if g.At(col, row) == Bridge {
				out = append(out, Chokepoint{
					Col:   col,
					Row:   row,
					Kind:  ChokeBridge,
					Score: bridgeScore,
				})
			}
		}
	}

	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if g.At(col, row) != Land {
				continue
			}
			score, ok := narrowScore(g, col, row)
			if !ok {
				continue
			}
			out = append(out, Chokepoint{
				Col:   col,
				Row:   row,
				Kind:  ChokeLandNarrow,
				Score: score,
			})
		}
	}

	return out
}

// narrowScore scores a Land zone walled on one axis and open on the other,
// returning false when the zone is not a corridor at all.
func narrowScore(g *TerrainGrid, col, row int) (float64, bool) {
	n := !passable(g.At(col, row-1))
	s := !passable(g.At(col, row+1))
	e := !passable(g.At(col+1, row))
	w := !passable(g.At(col-1, row))

	horizontal := n && s && passable(g.At(col-1, row)) && passable(g.At(col+1, row))
	vertical := e && w && passable(g.At(col, row-1)) && passable(g.At(col, row+1))

	if !horizontal && !vertical {
		return 0, false
	}

	// Deeper walls score higher: the corridor is harder to skirt.
	if horizontal {
		if !passable(g.At(col, row-2)) && !passable(g.At(col, row+2)) {
			return narrowDeepScore, true
		}
		return narrowShallowScore, true
	}
	if !passable(g.At(col-2, row)) && !passable(g.At(col+2, row)) {
		return narrowDeepScore, true
	}
	return narrowShallowScore, true
}

// RankChokepointsOnPath rescales scores by proximity to the shortest path from
// `from` to `to`, dropping unreachable chokes and sorting by descending score.
// Coordinates are grid [col,row]; an out-of-bounds or blocked endpoint yields no
// path information, so the input is returned unchanged.
func RankChokepointsOnPath(cps []Chokepoint, g *TerrainGrid, from, to [2]int) []Chokepoint {
	if g == nil || len(cps) == 0 {
		return cps
	}
	if !inBounds(g, from[0], from[1]) || !inBounds(g, to[0], to[1]) {
		return cps
	}
	if !passable(g.At(from[0], from[1])) || !passable(g.At(to[0], to[1])) {
		return cps
	}

	dist, parent := bfs(g, from)
	toIdx := to[1]*g.Cols + to[0]
	if dist[toIdx] < 0 {
		// Destination unreachable — drop unreachable chokes, leave the rest at
		// base score.
		out := make([]Chokepoint, 0, len(cps))
		for _, c := range cps {
			if dist[c.Row*g.Cols+c.Col] >= 0 {
				out = append(out, c)
			}
		}
		sortByScoreDesc(out)
		return out
	}

	onPath := make(map[int]bool)
	for cur := toIdx; cur != -1; cur = parent[cur] {
		onPath[cur] = true
		if cur == from[1]*g.Cols+from[0] {
			break
		}
	}

	out := make([]Chokepoint, 0, len(cps))
	for _, c := range cps {
		idx := c.Row*g.Cols + c.Col
		if dist[idx] < 0 {
			continue
		}
		if onPath[idx] {
			c.Score += pathBoost
		} else if anyNeighborOnPath(g, onPath, c.Col, c.Row) {
			c.Score += pathNearBoost
		}
		out = append(out, c)
	}
	sortByScoreDesc(out)
	return out
}

func inBounds(g *TerrainGrid, col, row int) bool {
	return col >= 0 && col < g.Cols && row >= 0 && row < g.Rows
}

// bfs returns zone distances from src over the passable subgraph plus a parent
// array; unreachable cells are -1 in both.
func bfs(g *TerrainGrid, src [2]int) (dist []int, parent []int) {
	n := g.Cols * g.Rows
	dist = make([]int, n)
	parent = make([]int, n)
	for i := range dist {
		dist[i] = -1
		parent[i] = -1
	}

	srcIdx := src[1]*g.Cols + src[0]
	dist[srcIdx] = 0
	queue := []int{srcIdx}

	dx := [4]int{1, -1, 0, 0}
	dy := [4]int{0, 0, 1, -1}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		col := cur % g.Cols
		row := cur / g.Cols
		for k := 0; k < 4; k++ {
			ncol := col + dx[k]
			nrow := row + dy[k]
			if !inBounds(g, ncol, nrow) || !passable(g.At(ncol, nrow)) {
				continue
			}
			nidx := nrow*g.Cols + ncol
			if dist[nidx] >= 0 {
				continue
			}
			dist[nidx] = dist[cur] + 1
			parent[nidx] = cur
			queue = append(queue, nidx)
		}
	}
	return dist, parent
}

func anyNeighborOnPath(g *TerrainGrid, onPath map[int]bool, col, row int) bool {
	dx := [4]int{1, -1, 0, 0}
	dy := [4]int{0, 0, 1, -1}
	for k := 0; k < 4; k++ {
		nc, nr := col+dx[k], row+dy[k]
		if !inBounds(g, nc, nr) {
			continue
		}
		if onPath[nr*g.Cols+nc] {
			return true
		}
	}
	return false
}

func sortByScoreDesc(cps []Chokepoint) {
	for i := 1; i < len(cps); i++ {
		for j := i; j > 0 && cps[j-1].Score < cps[j].Score; j-- {
			cps[j-1], cps[j] = cps[j], cps[j-1]
		}
	}
}
