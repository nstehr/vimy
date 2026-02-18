package ipc

// These constants must stay in sync with the C# MessageType enum in the OpenRA mod.
const (
	TypeHello     = "hello"
	TypeAck       = "ack"
	TypeGameState = "game_state"
)

type HelloMessage struct {
	Player string `json:"player"`
}

type AckMessage struct {
	Status string `json:"status"`
}
