package rules

import (
	"log/slog"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

// captureOrderEntry throttles repeat Capture orders. Same reason as
// sendAPCMove: a fresh Capture cancels the in-flight walk-and-capture, so an
// engineer re-ordered every tick never arrives.
type captureOrderEntry struct {
	Tick     int
	TargetID int
}

const captureOrderResend = 60

func getCaptureOrderState(memory map[string]any) map[int]captureOrderEntry {
	return memoryMap[int, captureOrderEntry](memory, "captureSent")
}

func ActionCaptureBuilding(env RuleEnv, conn CommandSender) error {
	target := env.NearestCapturable()
	if target == nil {
		return nil
	}
	engineers := env.IdleEngineers()
	if len(engineers) == 0 {
		return nil
	}
	// Closest first, so a just-unloaded engineer captures rather than being
	// re-loaded into another APC.
	eng, _ := nearestTo(engineers, target.X, target.Y)

	state := getCaptureOrderState(env.Memory)
	if prev, ok := state[eng.ID]; ok && prev.TargetID == target.ID && env.State.Tick-prev.Tick < captureOrderResend {
		return nil
	}
	state[eng.ID] = captureOrderEntry{Tick: env.State.Tick, TargetID: target.ID}

	slog.Debug("capturing building", "engineer", eng.ID, "target", target.ID, "type", target.Type)
	return conn.Send(ipc.TypeCapture, ipc.CaptureCommand{
		ActorID:  uint32(eng.ID),
		TargetID: uint32(target.ID),
	})
}
