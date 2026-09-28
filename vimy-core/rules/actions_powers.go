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
