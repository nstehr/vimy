package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

// Agent owns the decision-making for a single player session.
type Agent struct {
	Conn   *ipc.Connection
	Player string
}

func New(conn *ipc.Connection) *Agent {
	return &Agent{Conn: conn}
}

// HandleHello completes the handshake so the mod knows the bridge is ready.
func (a *Agent) HandleHello(env ipc.Envelope) (*ipc.Envelope, error) {
	var hello ipc.HelloMessage
	if err := json.Unmarshal(env.Data, &hello); err != nil {
		return nil, fmt.Errorf("unmarshal hello: %w", err)
	}

	a.Player = hello.Player
	slog.Info("player identified", "player", a.Player)

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}

func (a *Agent) HandleGameState(env ipc.Envelope) (*ipc.Envelope, error) {
	var gs GameState
	if err := json.Unmarshal(env.Data, &gs); err != nil {
		return nil, fmt.Errorf("unmarshal GameState: %w", err)
	}

	slog.Info("game state received",
		"player", gs.Player.Name,
		"tick", gs.Tick,
		"cash", gs.Player.Cash,
		"resources", gs.Player.Resources,
		"power", fmt.Sprintf("%d/%d (%s)", gs.Player.PowerDrained, gs.Player.PowerProvided, gs.Player.PowerState),
		"buildings", len(gs.Buildings),
		"units", len(gs.Units),
		"enemies", len(gs.Enemies),
		"queues", len(gs.ProductionQueues),
	)

	for _, u := range gs.Units {
		slog.Info("unit",
			"id", u.ID,
			"type", u.Type,
			"pos", fmt.Sprintf("(%d,%d)", u.X, u.Y),
			"hp", fmt.Sprintf("%d/%d", u.HP, u.MaxHP),
			"idle", u.Idle,
		)
	}

	for _, b := range gs.Buildings {
		slog.Info("building",
			"id", b.ID,
			"type", b.Type,
			"pos", fmt.Sprintf("(%d,%d)", b.X, b.Y),
			"hp", fmt.Sprintf("%d/%d", b.HP, b.MaxHP),
		)
	}

	for _, e := range gs.Enemies {
		slog.Info("enemy",
			"id", e.ID,
			"owner", e.Owner,
			"type", e.Type,
			"pos", fmt.Sprintf("(%d,%d)", e.X, e.Y),
			"hp", fmt.Sprintf("%d/%d", e.HP, e.MaxHP),
		)
	}

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}
