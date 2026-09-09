package model

// TerrainType classifies a coarse grid zone. Ore is deliberately excluded
// so the AI must scout for resources like a human player.
type TerrainType byte

const (
	Land   TerrainType = 0 // passable ground
	Water  TerrainType = 1 // naval only
	Cliff  TerrainType = 2 // impassable (rock, tree, wall)
	Bridge TerrainType = 3 // land corridor over water (chokepoint)
)

// TerrainGrid is a fixed 32x32 grid whatever the map size, so each zone covers
// CellW x CellH map cells.
type TerrainGrid struct {
	Cols  int           // grid columns (typically 32)
	Rows  int           // grid rows (typically 32)
	CellW int           // map cells per grid column
	CellH int           // map cells per grid row
	Grid  []TerrainType // row-major: Grid[row*Cols + col]
}

// At returns Land for out-of-bounds coordinates.
func (g *TerrainGrid) At(col, row int) TerrainType {
	if col < 0 || col >= g.Cols || row < 0 || row >= g.Rows {
		return Land
	}
	return g.Grid[row*g.Cols+col]
}

// AtMapPos resolves map coordinates against the grid, Land when out of bounds.
func (g *TerrainGrid) AtMapPos(mapX, mapY int) TerrainType {
	if g.CellW <= 0 || g.CellH <= 0 {
		return Land
	}
	col := mapX / g.CellW
	row := mapY / g.CellH
	return g.At(col, row)
}

// ZoneCenter is the map position at the middle of zone (col, row).
func (g *TerrainGrid) ZoneCenter(col, row int) (int, int) {
	x := col*g.CellW + g.CellW/2
	y := row*g.CellH + g.CellH/2
	return x, y
}

// HasWater returns true if any zone in the grid is classified as Water.
func (g *TerrainGrid) HasWater() bool {
	for _, t := range g.Grid {
		if t == Water {
			return true
		}
	}
	return false
}
