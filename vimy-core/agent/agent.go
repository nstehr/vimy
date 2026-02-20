package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// Agent owns the decision-making for a single player session.
type Agent struct {
	Conn   *ipc.Connection
	Player string
	Engine *rules.Engine
}

func New(conn *ipc.Connection, engine *rules.Engine) *Agent {
	return &Agent{Conn: conn, Engine: engine}
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
	var gs model.GameState
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

	if err := a.Engine.Evaluate(gs, a.Conn); err != nil {
		slog.Error("rule engine error", "error", err)
	}

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}
