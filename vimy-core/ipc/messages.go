package ipc

// These constants must stay in sync with the C# MessageType enum in the OpenRA mod.
const (
	TypeHello     = "hello"
	TypeAck       = "ack"
	TypeGameState = "game_state"
	TypeGameEnd   = "game_end"
)

type HelloMessage struct {
	Player  string       `json:"player"`
	Faction string       `json:"faction"`
	Terrain *TerrainData `json:"terrain,omitempty"`
}

// TerrainData carries the coarse terrain grid from the C# mod.
// Optional — if absent the sidecar continues without terrain awareness.
type TerrainData struct {
	Cols  int   `json:"cols"`
	Rows  int   `json:"rows"`
	CellW int   `json:"cellW"`
	CellH int   `json:"cellH"`
	Grid  []int `json:"grid"`
}

type GameEndMessage struct {
	Winner string `json:"winner"` // player name of the winner
}

type AckMessage struct {
	Status string `json:"status"`
}
