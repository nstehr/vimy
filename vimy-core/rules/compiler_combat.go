package rules

import "fmt"

// addCombatRules emits rules for defense squads, ground/air/naval attack
// squads, superweapon firing, and airfield support powers. It also computes
// attackPriority and activationThreshold which are reused by addMicroRules.
func (c *doctrineCompiler) addCombatRules() {
	// --- Defense behavior ---

	defendPriority := lerp(350, 500, c.d.GroundDefensePriority)

	// High defense priority: reserve a persistent squad so defenders aren't
	// poached by attack rules between engagements.
	if c.d.GroundDefensePriority > DoctrineSignificant {
		defenseSize := lerp(2, 5, c.d.GroundDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "form-defense-squad",
			Priority:     defendPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`(!SquadExists("ground-defense") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-defense") && len(UnassignedIdleGround()) >= 1)`, defenseSize),
			Action:       FormSquad("ground-defense", "ground", defenseSize, "defend"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-defend-base",
			Priority:     defendPriority,
			Category:     "combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("ground-defense") && SquadIdleCount("ground-defense") > 0 && BaseUnderAttack()`,
			Action:       SquadDefend("ground-defense"),
		})
	} else {
		// Low defense: no reserved squad, just scramble all idle ground units.
		defendMinUnits := lerp(3, 1, c.d.GroundDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "defend-base",
			Priority:     defendPriority,
			Category:     "combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`BaseUnderAttack() && len(IdleGroundUnits()) >= %d`, defendMinUnits),
			Action:       ActionDefendBase,
		})
	}

	airDefendPriority := lerp(350, 500, c.d.AirDefensePriority)
	c.rules = append(c.rules, &Rule{
		Name:         "defend-base-air",
		Priority:     airDefendPriority,
		Category:     "air_combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleCombatAircraft()) > 0`,
		Action:       ActionAirDefendBase,
	})

	// --- Ground attack ---

	c.attackPriority = lerp(200, 400, c.d.Aggression)
	c.activationThreshold = lerpf(0.6, 1.0, 1.0-c.d.Aggression)

	c.rules = append(c.rules, &Rule{
		Name:         "form-ground-attack",
		Priority:     c.attackPriority + SquadFormBonus,
		Category:     "squad_form",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`(!SquadExists("ground-attack") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-attack") && len(UnassignedIdleGround()) >= 1)`, c.d.GroundAttackGroupSize),
		Action:       FormSquad("ground-attack", "ground", c.d.GroundAttackGroupSize, "attack"),
	})

	c.rules = append(c.rules, &Rule{
		Name:         "squad-attack",
		Priority:     c.attackPriority,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && (BestGroundTarget() != nil || NearestEnemy() != nil)`, c.activationThreshold),
		Action:       SquadAttackMove("ground-attack"),
	})

	// Re-engage idle squad members already in the field — no ratio gate.
	// The main squad-attack rule handles the initial coordinated launch;
	// this handles the common case where some members finish their order
	// while others are still fighting.
	c.rules = append(c.rules, &Rule{
		Name:         "squad-reengage",
		Priority:     c.attackPriority - ReengageDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `SquadExists("ground-attack") && SquadIdleCount("ground-attack") > 0 && (BestGroundTarget() != nil || NearestEnemy() != nil)`,
		Action:       SquadAttackMove("ground-attack"),
	})

	// Fallback: attack last-known enemy base when fog of war hides all enemies.
	c.rules = append(c.rules, &Rule{
		Name:         "squad-attack-known-base",
		Priority:     c.attackPriority - KnownBaseDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, c.activationThreshold),
		Action:       SquadAttackKnownBase("ground-attack", c.d.Aggression),
	})

	// --- Air attack ---

	if c.d.AirWeight > DoctrineEnabled {
		airAttackPriority := lerp(200, 400, c.d.Aggression) - AirDomainOffset

		c.rules = append(c.rules, &Rule{
			Name:         "form-air-attack",
			Priority:     airAttackPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`(!SquadExists("air-attack") && len(UnassignedIdleAir()) >= %d) || (SquadNeedsReinforcement("air-attack") && len(UnassignedIdleAir()) >= 1)`, c.d.AirAttackGroupSize),
			Action:       FormSquad("air-attack", "air", c.d.AirAttackGroupSize, "attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-attack",
			Priority:     airAttackPriority,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && BestAirTarget() != nil`, c.activationThreshold),
			Action:       SquadAirStrike("air-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-reengage",
			Priority:     airAttackPriority - ReengageDiscount,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("air-attack") && SquadIdleCount("air-attack") > 0 && BestAirTarget() != nil`,
			Action:       SquadAirStrike("air-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-attack-known-base",
			Priority:     airAttackPriority - KnownBaseDiscount,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, c.activationThreshold),
			Action:       SquadAttackKnownBase("air-attack", c.d.Aggression),
		})
	}

	// --- Naval attack ---

	if c.d.NavalWeight > DoctrineEnabled {
		navalAttackPriority := lerp(200, 400, c.d.Aggression) - NavalDomainOffset

		c.rules = append(c.rules, &Rule{
			Name:         "form-naval-attack",
			Priority:     navalAttackPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && ((!SquadExists("naval-attack") && len(UnassignedIdleNaval()) >= %d) || (SquadNeedsReinforcement("naval-attack") && len(UnassignedIdleNaval()) >= 1))`, c.d.NavalAttackGroupSize),
			Action:       FormSquad("naval-attack", "naval", c.d.NavalAttackGroupSize, "attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-attack",
			Priority:     navalAttackPriority,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && SquadExists("naval-attack") && SquadReadyRatio("naval-attack") >= %.2f && NearestEnemy() != nil`, c.activationThreshold),
			Action:       SquadAttackMove("naval-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-reengage",
			Priority:     navalAttackPriority - ReengageDiscount,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: `MapHasWater() && SquadExists("naval-attack") && SquadIdleCount("naval-attack") > 0 && NearestEnemy() != nil`,
			Action:       SquadAttackMove("naval-attack"),
		})

		// Fallback: attack last-known enemy base when fog hides all enemies.
		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-attack-known-base",
			Priority:     navalAttackPriority - KnownBaseDiscount,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && SquadExists("naval-attack") && SquadReadyRatio("naval-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, c.activationThreshold),
			Action:       SquadAttackKnownBase("naval-attack", c.d.Aggression),
		})
	}

	// --- Superweapon fire ---

	if c.d.SuperweaponPriority > DoctrineEnabled {
		c.rules = append(c.rules, &Rule{
			Name:         "fire-nuke",
			Priority:     880,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("NukePowerInfoOrder")`,
			Action:       ActionFireNuke,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "fire-iron-curtain",
			Priority:     870,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`SupportPowerReady("GrantExternalConditionPowerInfoOrder") && len(IdleGroundUnits()) >= %d`, IronCurtainMinUnits),
			Action:       ActionFireIronCurtain,
		})
	}

	// --- Airfield support powers ---

	if c.d.AirWeight > DoctrineEnabled {
		c.rules = append(c.rules, &Rule{
			Name:         "fire-spy-plane",
			Priority:     860,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietSpyPlane") && !HasEnemyIntel()`,
			Action:       ActionFireSpyPlane,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "fire-spy-plane-update",
			Priority:     250,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietSpyPlane") && HasEnemyIntel() && !EnemiesVisible()`,
			Action:       ActionFireSpyPlane,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "fire-paratroopers",
			Priority:     855,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietParatroopers") && (HasEnemyIntel() || EnemiesVisible())`,
			Action:       ActionFireParatroopers,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "fire-parabombs",
			Priority:     845,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("UkraineParabombs") && (HasEnemyIntel() || EnemiesVisible())`,
			Action:       ActionFireParabombs,
		})
	}
}
