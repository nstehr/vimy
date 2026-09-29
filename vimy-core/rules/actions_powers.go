package rules

import (
	"log/slog"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

func ActionFireNuke(env RuleEnv, conn CommandSender) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil {
		x, y = enemy.X, enemy.Y
	} else {
		x, y = env.MapWidth()/2, env.MapHeight()/2
	}
	recordSuperweaponFire(env, "nuke")
	slog.Info("firing nuke", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "NukePowerInfoOrder",
		X:        x,
		Y:        y,
	})
}

func ActionFireIronCurtain(env RuleEnv, conn CommandSender) error {
	x, y := env.GroundUnitCentroid()
	recordSuperweaponFire(env, "iron_curtain")
	slog.Info("firing iron curtain on own units", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "GrantExternalConditionPowerInfoOrder",
		X:        x,
		Y:        y,
	})
}

// ActionFireGPS launches the satellite, which reveals the whole map permanently.
//
// The only non-superweapon support power an Allied faction has, and nothing
// fired it: every other registered power -- spy plane, paratroopers, parabombs
// -- lives on AFLD, the Soviet airfield, so an Allied game has three power rules
// that evaluate about 1981 times and can never succeed. GPS lives on ATEK, which
// Vimy already builds one to four of per game at 1500 credits, so it was paying
// for the building and leaving the power on it.
//
// It is worth firing because the two defect-class strike blockers are vision
// problems. In game 190 not-building reached 287 and blind-at-base 25:
// not-building means something was visible and the best of it was a unit, so no
// building was in view at all, and squadStructureTarget only falls back to a
// remembered structure once already within strike reach. A permanent reveal
// turns every enemy structure into a live target instead of a memory, and feeds
// the threat field that the approach router scores corridors against.
//
// No target cell matters -- the reveal is global -- but the order still carries
// one, so it gets the enemy base when known and the map centre otherwise, the
// same as the spy plane.
//
// OneShot with a ChargeInterval of 12000, so support-power-ready gates it to a
// single launch and there is nothing to debounce here.
func ActionFireGPS(env RuleEnv, conn CommandSender) error {
	x, y := env.MapWidth()/2, env.MapHeight()/2
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	}
	recordSuperweaponFire(env, "gps")
	slog.Info("launching gps satellite", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "GpsPowerInfoOrder",
		X:        x,
		Y:        y,
	})
}

func ActionFireSpyPlane(env RuleEnv, conn CommandSender) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else {
		x, y = env.MapWidth()/2, env.MapHeight()/2
	}
	recordSuperweaponFire(env, "spy_plane")
	slog.Info("firing spy plane", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "SovietSpyPlane",
		X:        x,
		Y:        y,
	})
}

func ActionFireParatroopers(env RuleEnv, conn CommandSender) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil && env.IsLandAt(base.X, base.Y) {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil && env.IsLandAt(enemy.X, enemy.Y) {
		x, y = enemy.X, enemy.Y
	} else {
		return nil // no valid land target
	}
	recordSuperweaponFire(env, "paratroopers")
	slog.Info("firing paratroopers", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "SovietParatroopers",
		X:        x,
		Y:        y,
	})
}

func ActionFireParabombs(env RuleEnv, conn CommandSender) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil && env.IsLandAt(base.X, base.Y) {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil && env.IsLandAt(enemy.X, enemy.Y) {
		x, y = enemy.X, enemy.Y
	} else {
		return nil // no valid land target
	}
	recordSuperweaponFire(env, "parabombs")
	slog.Info("firing parabombs", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "UkraineParabombs",
		X:        x,
		Y:        y,
	})
}
