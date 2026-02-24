package model

import "testing"

func TestTerrainGridAt(t *testing.T) {
	grid := &TerrainGrid{
		Cols:  4,
		Rows:  4,
		CellW: 8,
		CellH: 8,
		Grid: []TerrainType{
			Land, Land, Water, Water,
			Land, Land, Water, Water,
			Cliff, Bridge, Land, Land,
			Cliff, Land, Land, Land,
		},
	}

	tests := []struct {
		col, row int
		want     TerrainType
	}{
		{0, 0, Land},
		{2, 0, Water},
		{0, 2, Cliff},
		{1, 2, Bridge},
		{3, 3, Land},
	}
	for _, tc := range tests {
		got := grid.At(tc.col, tc.row)
		if got != tc.want {
			t.Errorf("At(%d, %d) = %d, want %d", tc.col, tc.row, got, tc.want)
		}
	}
}

func TestTerrainGridAtOutOfBounds(t *testing.T) {
	grid := &TerrainGrid{
		Cols:  2,
		Rows:  2,
		CellW: 4,
		CellH: 4,
		Grid:  []TerrainType{Water, Water, Water, Water},
	}

	// Out-of-bounds should return Land (safe default).
	if got := grid.At(-1, 0); got != Land {
		t.Errorf("At(-1, 0) = %d, want Land", got)
	}
	if got := grid.At(0, -1); got != Land {
		t.Errorf("At(0, -1) = %d, want Land", got)
	}
	if got := grid.At(2, 0); got != Land {
		t.Errorf("At(2, 0) = %d, want Land", got)
	}
	if got := grid.At(0, 2); got != Land {
		t.Errorf("At(0, 2) = %d, want Land", got)
	}
}

func TestTerrainGridAtMapPos(t *testing.T) {
	grid := &TerrainGrid{
		Cols:  4,
		Rows:  4,
		CellW: 8,
		CellH: 8,
		Grid: []TerrainType{
			Land, Land, Water, Water,
			Land, Land, Water, Water,
			Cliff, Bridge, Land, Land,
			Cliff, Land, Land, Land,
		},
	}

	tests := []struct {
		mapX, mapY int
		want       TerrainType
	}{
		{0, 0, Land},     // col=0, row=0
		{4, 0, Land},     // col=0, row=0 (just inside)
		{16, 0, Water},   // col=2, row=0
		{24, 16, Land},   // col=3, row=2
		{0, 16, Cliff},   // col=0, row=2
		{8, 16, Bridge},  // col=1, row=2
	}
	for _, tc := range tests {
		got := grid.AtMapPos(tc.mapX, tc.mapY)
		if got != tc.want {
			t.Errorf("AtMapPos(%d, %d) = %d, want %d", tc.mapX, tc.mapY, got, tc.want)
		}
	}
}

func TestTerrainGridAtMapPosZeroCells(t *testing.T) {
	grid := &TerrainGrid{
		Cols:  2,
		Rows:  2,
		CellW: 0,
		CellH: 0,
		Grid:  []TerrainType{Water, Water, Water, Water},
	}
	// Zero cell size should return Land (safe default).
	if got := grid.AtMapPos(5, 5); got != Land {
		t.Errorf("AtMapPos with zero cells = %d, want Land", got)
	}
}

func TestTerrainGridZoneCenter(t *testing.T) {
	grid := &TerrainGrid{
		Cols:  4,
		Rows:  4,
		CellW: 8,
		CellH: 8,
	}

	x, y := grid.ZoneCenter(0, 0)
	if x != 4 || y != 4 {
		t.Errorf("ZoneCenter(0,0) = (%d,%d), want (4,4)", x, y)
	}

	x, y = grid.ZoneCenter(1, 2)
	if x != 12 || y != 20 {
		t.Errorf("ZoneCenter(1,2) = (%d,%d), want (12,20)", x, y)
	}
}

func TestTerrainGridHasWater(t *testing.T) {
	noWater := &TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []TerrainType{Land, Land, Cliff, Land},
	}
	if noWater.HasWater() {
		t.Error("HasWater() should be false for land-only grid")
	}

	withWater := &TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []TerrainType{Land, Water, Cliff, Land},
	}
	if !withWater.HasWater() {
		t.Error("HasWater() should be true for grid with water")
	}
}
