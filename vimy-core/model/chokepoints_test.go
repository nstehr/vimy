package model

import "testing"

func TestFindChokepointsOpenField(t *testing.T) {
	g := &TerrainGrid{
		Cols: 4, Rows: 4, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Land, Land, Land, Land,
			Land, Land, Land, Land,
			Land, Land, Land, Land,
			Land, Land, Land, Land,
		},
	}
	if got := FindChokepoints(g); len(got) != 0 {
		t.Errorf("open field: got %d chokepoints, want 0 (%+v)", len(got), got)
	}
}

func TestFindChokepointsSingleBridge(t *testing.T) {
	// Same fixture as TestTerrainGridAt — bridge sits at (1,2).
	g := &TerrainGrid{
		Cols: 4, Rows: 4, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Land, Land, Water, Water,
			Land, Land, Water, Water,
			Cliff, Bridge, Land, Land,
			Cliff, Land, Land, Land,
		},
	}
	got := FindChokepoints(g)
	if len(got) != 1 {
		t.Fatalf("got %d chokepoints, want 1 (%+v)", len(got), got)
	}
	if got[0].Kind != ChokeBridge || got[0].Col != 1 || got[0].Row != 2 {
		t.Errorf("got %+v, want bridge at (1,2)", got[0])
	}
	if got[0].Score < 0.8 {
		t.Errorf("bridge score %.2f, want >= 0.8", got[0].Score)
	}
}

func TestFindChokepointsLandGap(t *testing.T) {
	// Row of land pinched between two rows of water on each side. Every land
	// cell in row 2 should be flagged as a deep narrow corridor.
	g := &TerrainGrid{
		Cols: 5, Rows: 5, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Water, Water, Water, Water, Water,
			Water, Water, Water, Water, Water,
			Land, Land, Land, Land, Land,
			Water, Water, Water, Water, Water,
			Water, Water, Water, Water, Water,
		},
	}
	got := FindChokepoints(g)
	if len(got) != 5 {
		t.Fatalf("got %d chokepoints, want 5 (%+v)", len(got), got)
	}
	for _, c := range got {
		if c.Kind != ChokeLandNarrow {
			t.Errorf("choke %+v: want ChokeLandNarrow", c)
		}
		if c.Row != 2 {
			t.Errorf("choke %+v: want row=2", c)
		}
		if c.Score < 0.65 {
			t.Errorf("choke %+v: deep-wall score should be ~0.7, got %.2f", c, c.Score)
		}
	}
}

func TestFindChokepointsVerticalCliffPinch(t *testing.T) {
	// Vertical land corridor between two cliff walls.
	g := &TerrainGrid{
		Cols: 5, Rows: 5, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Land, Land, Land, Land, Land,
			Land, Cliff, Land, Cliff, Land,
			Land, Cliff, Land, Cliff, Land,
			Land, Cliff, Land, Cliff, Land,
			Land, Land, Land, Land, Land,
		},
	}
	got := FindChokepoints(g)
	// Exactly the three middle-column land cells (2,1),(2,2),(2,3) should
	// match. The ends are surrounded by open land.
	var narrow []Chokepoint
	for _, c := range got {
		if c.Kind == ChokeLandNarrow {
			narrow = append(narrow, c)
		}
	}
	if len(narrow) != 3 {
		t.Fatalf("got %d narrow chokes, want 3 (%+v)", len(narrow), narrow)
	}
	for _, c := range narrow {
		if c.Col != 2 {
			t.Errorf("narrow choke %+v: want col=2", c)
		}
	}
}

func TestRankChokepointsOnPathPrefersOnPath(t *testing.T) {
	// Two bridges, only (2,1) lies on the shortest passable path from (0,0)
	// to (4,4). (0,3) is reachable but off the direct route.
	g := &TerrainGrid{
		Cols: 5, Rows: 5, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Land, Land, Land, Land, Land,
			Water, Water, Bridge, Water, Water,
			Water, Land, Land, Land, Water,
			Bridge, Land, Land, Land, Water,
			Land, Land, Land, Land, Land,
		},
	}
	cps := FindChokepoints(g)
	ranked := RankChokepointsOnPath(cps, g, [2]int{0, 0}, [2]int{4, 4})

	var onPath, offPath *Chokepoint
	for i, c := range ranked {
		if c.Col == 2 && c.Row == 1 {
			onPath = &ranked[i]
		}
		if c.Col == 0 && c.Row == 3 {
			offPath = &ranked[i]
		}
	}
	if onPath == nil || offPath == nil {
		t.Fatalf("expected both bridges in ranked output, got %+v", ranked)
	}
	if onPath.Score <= offPath.Score {
		t.Errorf("on-path bridge score %.2f should exceed off-path %.2f", onPath.Score, offPath.Score)
	}
	if ranked[0].Col != 2 || ranked[0].Row != 1 {
		t.Errorf("ranked[0] = %+v, want on-path bridge at (2,1)", ranked[0])
	}
}

func TestRankChokepointsOnPathDropsUnreachable(t *testing.T) {
	// Bridge (2,2) is fully walled off by cliffs — reachable by nobody.
	// Outer ring is open so (0,0)→(4,4) still connects.
	g := &TerrainGrid{
		Cols: 5, Rows: 5, CellW: 8, CellH: 8,
		Grid: []TerrainType{
			Land, Land, Land, Land, Land,
			Land, Land, Cliff, Land, Land,
			Land, Cliff, Bridge, Cliff, Land,
			Land, Land, Cliff, Land, Land,
			Land, Land, Land, Land, Land,
		},
	}
	cps := FindChokepoints(g)
	hasIsolatedBridge := false
	for _, c := range cps {
		if c.Col == 2 && c.Row == 2 {
			hasIsolatedBridge = true
		}
	}
	if !hasIsolatedBridge {
		t.Fatalf("detector should find bridge at (2,2) before ranking; got %+v", cps)
	}

	ranked := RankChokepointsOnPath(cps, g, [2]int{0, 0}, [2]int{4, 4})
	for _, c := range ranked {
		if c.Col == 2 && c.Row == 2 {
			t.Errorf("unreachable bridge should be dropped, got %+v in ranked output", c)
		}
	}
}

func TestFindChokepointsNilGrid(t *testing.T) {
	if got := FindChokepoints(nil); got != nil {
		t.Errorf("nil grid: got %+v, want nil", got)
	}
}
