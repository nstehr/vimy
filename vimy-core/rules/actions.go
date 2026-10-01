package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// ActionDefendBase answers a threat at home, drawing on the least committed
// units that will do.
//
// This replaces four rules that all did the same thing with different triggers
// and different unit pools — scramble-base-defense, emergency-base-defense,
// defend-base and squad-defend-base — plus defend-critical-building, which was
// the same again with an override. One of them said so in its own because:
// "above scramble-base-defense, which does the same thing with a looser
// condition". Nobody designed that; it accreted.
//
// The cost was measurable. Game 143 spent 2143 defensive acts against 248
// attack acts, 8.6 to 1, and a squad setting out lost a third of itself on the
// way: 4.5 members far from the target, 3.2 at mid range with the furthest 41
// cells adrift, 2.9 on arrival. Four separate rules were reaching into the
// same pool with no idea the others existed, and narrowing any one of them
// just widened the next — emergency-base-defense, then
// defend-critical-building, then scramble-base-defense, three rounds of it.
//
// So the escalation lives in one place and is explicit. Take the garrison
// first. Take unassigned units next. Break into a committed assault only when
// core infrastructure is actually being hit, which is the one case where the
// old override was right: losing the base loses the game whatever the squad
// was doing.
func ActionDefendBase(env RuleEnv, conn CommandSender) error {
	// A damaged critical building picks the target and licenses the last tier.
	critical := env.nearestEnemyAttackingCritical()
	enemy := critical
	if enemy == nil {
		enemy = env.NearestEnemy()
	}
	if enemy == nil {
		return nil
	}

	units, tier := defenders(env, critical != nil)
	if len(units) == 0 {
		return nil
	}
	ids := make([]uint32, len(units))
	for i, u := range units {
		ids[i] = uint32(u.ID)
	}
	slog.Info("defending base", "count", len(ids), "tier", tier,
		"target", enemy.ID, "targetType", enemy.Type, "critical", critical != nil)
	recordDefendBase(env, tier, units, enemy.X, enemy.Y)
	return sendAttackMove(env, conn, ids, enemy.X, enemy.Y)
}

// defenders picks the least committed units that can answer a threat, and
// names the tier it had to reach for so the log can say how bad it got.
func defenders(env RuleEnv, criticalHit bool) ([]model.Unit, string) {
	// 1. The garrison. This is what a ground-defense squad is for.
	if squad := squadMembers(env, "ground-defense"); len(squad) > 0 {
		return squad, "garrison"
	}
	near := env.NearBaseGroundUnits()
	if len(near) == 0 {
		return nil, "none"
	}
	// 2. Anything near the base that no squad has claimed.
	if free := withoutAnySquad(env, near); len(free) > 0 {
		return free, "unassigned"
	}
	// 3. Anything not committed to an offensive — defensive squads included.
	if notAttacking := withoutAttackSquads(env, near); len(notAttacking) > 0 {
		return notAttacking, "reserves"
	}
	// 4. The assault itself, and only for infrastructure actually being hit.
	if criticalHit {
		return near, "assault recalled"
	}
	return nil, "none"
}

// squadMembers is a named squad's living members.
func squadMembers(env RuleEnv, name string) []model.Unit {
	sq, ok := getSquads(env.Memory)[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return nil
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	var out []model.Unit
	for _, u := range env.State.Units {
		if ids[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

// withoutAnySquad drops units rostered to any squad at all.
func withoutAnySquad(env RuleEnv, units []model.Unit) []model.Unit {
	claimed := make(map[int]bool)
	for _, sq := range getSquads(env.Memory) {
		for _, id := range sq.UnitIDs {
			claimed[id] = true
		}
	}
	out := make([]model.Unit, 0, len(units))
	for _, u := range units {
		if !claimed[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

// withoutAttackSquads drops units rostered to a squad with the attack role, so
// a base alarm draws on the garrison before the offensive.
func withoutAttackSquads(env RuleEnv, units []model.Unit) []model.Unit {
	committed := make(map[int]bool)
	for _, sq := range getSquads(env.Memory) {
		if !strings.EqualFold(sq.Role, "attack") {
			continue
		}
		for _, id := range sq.UnitIDs {
			committed[id] = true
		}
	}
	if len(committed) == 0 {
		return units
	}
	out := make([]model.Unit, 0, len(units))
	for _, u := range units {
		if !committed[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func ActionNavalDefendBase(env RuleEnv, conn CommandSender) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleNavalUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("naval defending base", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func ActionAirDefendBase(env RuleEnv, conn CommandSender) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	for _, u := range env.IdleCombatAircraft() {
		slog.Debug("air defend", "aircraft", u.ID, "target", enemy.ID)
		if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
			ActorID:  uint32(u.ID),
			TargetID: uint32(enemy.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func ActionAttackMoveIdleGroundUnits(env RuleEnv, conn CommandSender) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleGroundUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("attack-moving idle ground units", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func ActionAttackKnownBaseGround(env RuleEnv, conn CommandSender) error {
	base := env.NearestEnemyBase()
	if base == nil {
		return nil
	}
	idle := env.IdleGroundUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("ground attacking known enemy base", "count", len(ids), "owner", base.Owner, "x", base.X, "y", base.Y)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        base.X,
		Y:        base.Y,
	})
}

func ActionAirAttackEnemy(env RuleEnv, conn CommandSender) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	for _, u := range env.IdleCombatAircraft() {
		slog.Debug("air attack enemy", "aircraft", u.ID, "target", enemy.ID)
		if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
			ActorID:  uint32(u.ID),
			TargetID: uint32(enemy.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func ActionAirAttackKnownBase(env RuleEnv, conn CommandSender) error {
	base := env.NearestEnemyBase()
	if base == nil {
		return nil
	}
	aircraft := env.IdleCombatAircraft()
	if len(aircraft) == 0 {
		return nil
	}
	ids := make([]uint32, len(aircraft))
	for i, u := range aircraft {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("air attacking known enemy base", "count", len(ids), "owner", base.Owner, "x", base.X, "y", base.Y)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        base.X,
		Y:        base.Y,
	})
}

// memoryMap fetches, allocating on first touch, a typed table in engine memory.
// Backs the per-actor throttle/tracking maps.
func memoryMap[K comparable, V any](memory map[string]any, key string) map[K]V {
	m, _ := memory[key].(map[K]V)
	if m == nil {
		m = make(map[K]V)
		memory[key] = m
	}
	return m
}

// nearestTo returns the unit closest to (x, y) and the distance.
func nearestTo(units []model.Unit, x, y int) (model.Unit, float64) {
	best := units[0]
	bestDist := math.MaxFloat64
	for _, u := range units {
		dx := float64(u.X - x)
		dy := float64(u.Y - y)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = u
		}
	}
	return best, math.Sqrt(bestDist)
}

// Roomy enough to tolerate an APC stalling short of the building; the engineer
// walks the rest.
const unloadNearTargetCells = 10

func ActionNavalAttackEnemy(env RuleEnv, conn CommandSender) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleNavalUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("naval attacking enemy", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func GroundAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		idle := env.IdleGroundUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("attack-moving ground group", "count", n, "total_idle", len(idle), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

func GroundAttackKnownBaseGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		idle := env.IdleGroundUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("ground attacking known base (group)", "count", n, "total_idle", len(idle), "owner", base.Owner, "x", base.X, "y", base.Y)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        base.X,
			Y:        base.Y,
		})
	}
}

func AirAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		aircraft := env.IdleCombatAircraft()
		if len(aircraft) == 0 {
			return nil
		}
		n := min(maxUnits, len(aircraft))
		for i := range n {
			u := aircraft[i]
			slog.Debug("air attack enemy (group)", "aircraft", u.ID, "target", enemy.ID)
			if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
				ActorID:  uint32(u.ID),
				TargetID: uint32(enemy.ID),
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

func AirAttackKnownBaseGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		aircraft := env.IdleCombatAircraft()
		if len(aircraft) == 0 {
			return nil
		}
		n := min(maxUnits, len(aircraft))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(aircraft[i].ID)
		}
		slog.Debug("air attacking known base (group)", "count", n, "total_idle", len(aircraft), "owner", base.Owner, "x", base.X, "y", base.Y)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        base.X,
			Y:        base.Y,
		})
	}
}

func NavalAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		idle := env.IdleNavalUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("naval attacking enemy (group)", "count", n, "total_idle", len(idle), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

// unblockEgressResend — long enough for a normal move to complete, short
// enough to retry promptly when the blocker is pinned and can't leave.
const unblockEgressResend = 120
