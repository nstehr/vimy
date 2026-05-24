package model

import "testing"

func TestThreatFieldAddSourceFalloff(t *testing.T) {
	g := &TerrainGrid{Cols: 5, Rows: 5, CellW: 10, CellH: 10}
	f := NewThreatField(g)

	// Add a source at zone center (2,2) — map pos (25,25).
	f.AddSource(g, 25, 25, 1.0)

	if got := f.At(2, 2); got != 1.0 {
		t.Errorf("center: got %.2f, want 1.0", got)
	}
	if got := f.At(1, 2); got != 0.5 {
		t.Errorf("4-neighbor: got %.2f, want 0.5", got)
	}
	if got := f.At(1, 1); got < 0.33 || got > 0.34 {
		t.Errorf("diagonal: got %.3f, want ~0.333", got)
	}
	if got := f.At(0, 2); got != 0 {
		t.Errorf("2 cells away: got %.2f, want 0", got)
	}
}

func TestApproachPathPrefersLowThreat(t *testing.T) {
	// Open 5x5 land grid. Threat cluster on the direct row-2 path forces the
	// weighted BFS to detour along the edge.
	g := &TerrainGrid{
		Cols: 5, Rows: 5, CellW: 10, CellH: 10,
		Grid: []TerrainType{
			Land, Land, Land, Land, Land,
			Land, Land, Land, Land, Land,
			Land, Land, Land, Land, Land,
			Land, Land, Land, Land, Land,
			Land, Land, Land, Land, Land,
		},
	}
	f := NewThreatField(g)
	// Paint heavy threat on the middle row so the direct route is expensive.
	f.AddSource(g, 25, 25, 10.0) // zone (2,2)

	path := ApproachPath(g, f, [2]int{0, 2}, [2]int{4, 2}, 5.0)
	if path == nil {
		t.Fatal("expected a path, got nil")
	}
	for _, p := range path {
		if p[0] == 2 && p[1] == 2 {
			t.Errorf("weighted path went through hot zone %v: %v", p, path)
		}
	}
}

func TestApproachPathFallsBackToShortestWithoutThreat(t *testing.T) {
	g := &TerrainGrid{
		Cols: 3, Rows: 3, CellW: 10, CellH: 10,
		Grid: []TerrainType{
			Land, Land, Land,
			Land, Land, Land,
			Land, Land, Land,
		},
	}
	path := ApproachPath(g, nil, [2]int{0, 0}, [2]int{2, 2}, 0)
	if len(path) != 5 {
		t.Errorf("expected 5-zone Manhattan path, got %d (%v)", len(path), path)
	}
}

func TestApproachPathUnreachable(t *testing.T) {
	// Column of water fully separates left and right halves.
	g := &TerrainGrid{
		Cols: 5, Rows: 3, CellW: 10, CellH: 10,
		Grid: []TerrainType{
			Land, Land, Water, Land, Land,
			Land, Land, Water, Land, Land,
			Land, Land, Water, Land, Land,
		},
	}
	if path := ApproachPath(g, nil, [2]int{0, 1}, [2]int{4, 1}, 0); path != nil {
		t.Errorf("expected nil path, got %v", path)
	}
}
