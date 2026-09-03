package rules

import "fmt"

// addCombatRules emits rules for defense squads, ground/air/naval attack
// squads, superweapon firing, and airfield support powers. It also computes
// attackPriority and activationThreshold which are reused by addMicroRules.
func (c *doctrineCompiler) addCombatRules() {
	// --- Defense behavior ---

	defendPriority := lerp(350, 500, c.d.GroundDefensePriority)

	// Pre-compute the attack-squad creation threshold so the defense rule
	// can reserve units only when there's also enough surplus for attack.
	// Mirrors the floor used in form-ground-attack below.
	formGroundAttackThreshold := c.d.GroundAttackGroupSize * 6 / 10
	if formGroundAttackThreshold < 3 {
		formGroundAttackThreshold = 3
	}

	// High defense priority: reserve a persistent squad so defenders aren't
	// poached by attack rules between engagements.
	if c.d.GroundDefensePriority > DoctrineSignificant {
		defenseSize := lerp(2, 5, c.d.GroundDefensePriority)
		// Only pre-position the defense squad when (a) the base is actually
		// under attack — units must defend now — or (b) we have enough
		// unassigned ground to ALSO feed form-ground-attack at its threshold.
		// Without this, a small army (post-attrition or early game) gets
		// fully poached into a defend role that has nothing to do
		// (squad-defend-base only fires on BaseUnderAttack), starving attack
		// formation forever. Game 16 (vimy-sde): mammoth tank + medium tanks
		// observed sitting idle next to base while no enemy threat existed.
		surplusThreshold := defenseSize + formGroundAttackThreshold
		c.rules = append(c.rules, &Rule{
			Name:         "form-defense-squad",
			Priority:     defendPriority + SquadFormBonus,
			Category:     "squad-form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`((!SquadExists("ground-defense") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-defense") && len(UnassignedIdleGround()) >= 1)) && (BaseUnderAttack() || len(UnassignedIdleGround()) >= %d)`, defenseSize, surplusThreshold),
			Action:       FormSquad("ground-defense", "ground", defenseSize, "defend"),
			ActionSrc:    actionSrc("form-squad", "ground-defense", "ground", defenseSize, "defend"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-defend-base",
			Priority:     defendPriority,
			Category:     "combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("ground-defense") && SquadIdleCount("ground-defense") > 0 && BaseUnderAttack()`,
			Action:       SquadDefend("ground-defense"),
			ActionSrc:    actionSrc("squad-defend", "ground-defense"),
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
		Category:     "air-combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleCombatAircraft()) > 0`,
		Action:       ActionAirDefendBase,
	})

	// --- Ground attack ---

	c.attackPriority = lerp(200, 400, c.d.Aggression)
	// commit_ratio doctrine knob: LLM-set squad activation threshold. When
	// non-zero, it overrides the aggression-derived default (lerp 0.6-1.0
	// inverse of aggression). Lets rush doctrines commit at 0.3 (attack fast
	// with partial squad) while turtle doctrines commit at 0.8 (full squad
	// or nothing) — same knob, different values per style.
	c.activationThreshold = lerpf(0.6, 1.0, 1.0-c.d.Aggression)
	if c.d.CommitRatio > 0 {
		c.activationThreshold = c.d.CommitRatio
	}

	// Squad-creation threshold: 60% of the requested group size, floor 3
	// (computed above as formGroundAttackThreshold so the defense rule can
	// gate on it too). The full GroundAttackGroupSize still gates
	// SquadReadyRatio for the actual attack commit; the squad just needs
	// to exist for that to start counting. Game 15 (vimy-a8e): LLM
	// defaulted GroundAttackGroupSize=10 in 105 of 200 doctrines; defense
	// squad took 2-5 units first; with total ground forces under 15
	// (common post-attrition), only 3-5 were unassigned — below the
	// 10-unit creation threshold. Squad never spawned, attack never
	// happened. Lowering the creation gate lets a partial squad form and
	// the existing reinforcement path then tops it up via
	// SquadNeedsReinforcement.
	c.rules = append(c.rules, &Rule{
		Name:         "form-ground-attack",
		Priority:     c.attackPriority + SquadFormBonus,
		Category:     "squad-form",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`(!SquadExists("ground-attack") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-attack") && len(UnassignedIdleGround()) >= 1)`, formGroundAttackThreshold),
		Action:       FormSquad("ground-attack", "ground", c.d.GroundAttackGroupSize, "attack"),
		ActionSrc:    actionSrc("form-squad", "ground-attack", "ground", c.d.GroundAttackGroupSize, "attack"),
	})

	// Ground-attack action choice: squad-attack (chase visible enemy) and
	// squad-attack-known-base (press the enemy base) are mutually exclusive
	// via the "ground_attack_choice" category so we don't thrash between two
	// destinations. Priority order determines which wins on a given tick.
	//
	// Aggressive doctrines (aggression >= DoctrineSignificant) with known-base
	// intel PREFER the base attack over swatting local raiders — the base
	// defenses (pillbox, ground-defense squad, emergency-base-defense) handle
	// the raider while the ground-attack squad executes the strategic push.
	// Non-aggressive doctrines keep the historical priority (visible enemy
	// beats known base) so defensive postures don't over-commit forward.
	//
	// !EnemiesVisible removed from squad-attack-known-base (was blocking rush
	// deployments: game 49 had 22 first_contact events but only 2 known-base
	// firings — a persistent raider kept the visible-enemy check true and
	// the base attack never fired despite intel being present).
	knownBasePriority := c.attackPriority - KnownBaseDiscount
	if c.d.Aggression >= DoctrineSignificant {
		knownBasePriority = c.attackPriority + 5 // outrank squad-attack
	}

	// base_defense_floor doctrine knob: when non-zero, the ground-attack
	// squad won't deploy until at least N static ground defenses exist. A
	// rush sets 0-1 (no floor), a balanced push sets 2-3, a turtle 6-8.
	// Prevents committing forward while the base is naked.
	baseDefenseFloorClause := ""
	if c.d.BaseDefenseFloor > 0 {
		baseDefenseFloorClause = fmt.Sprintf(` && (RoleCount("pillbox") + RoleCount("camo_pillbox") + RoleCount("turret") + RoleCount("flame_tower") + RoleCount("tesla_coil")) >= %d`, c.d.BaseDefenseFloor)
	}

	c.rules = append(c.rules, &Rule{
		Name:         "squad-attack",
		Priority:     c.attackPriority,
		Category:     "ground-attack-choice",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && (BestGroundTarget() != nil || NearestEnemy() != nil)%s`, c.activationThreshold, baseDefenseFloorClause),
		Action:       SquadAttackMove("ground-attack"),
		ActionSrc:    actionSrc("squad-attack-move", "ground-attack"),
	})

	// Re-engage idle squad members already in the field — no ratio gate,
	// non-exclusive so it fires alongside whichever ground_attack_choice
	// rule won this tick (catches stragglers finishing an order while the
	// squad presses forward).
	c.rules = append(c.rules, &Rule{
		Name:         "squad-reengage",
		Priority:     c.attackPriority - ReengageDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `SquadExists("ground-attack") && SquadIdleCount("ground-attack") > 0 && (BestGroundTarget() != nil || NearestEnemy() != nil)`,
		Action:       SquadAttackMove("ground-attack"),
		ActionSrc:    actionSrc("squad-attack-move", "ground-attack"),
	})

	c.rules = append(c.rules, &Rule{
		Name:         "squad-attack-known-base",
		Priority:     knownBasePriority,
		Category:     "ground-attack-choice",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && HasEnemyIntel()%s`, c.activationThreshold, baseDefenseFloorClause),
		Action:       SquadAttackKnownBase("ground-attack", c.d.Aggression),
		ActionSrc:    actionSrc("squad-attack-known-base", "ground-attack", c.d.Aggression),
	})

	// --- Air attack ---

	if c.d.AirWeight > DoctrineEnabled {
		airAttackPriority := lerp(200, 400, c.d.Aggression) - AirDomainOffset

		// Same partial-roster fix as form-ground-attack (vimy-a8e).
		airFormThreshold := c.d.AirAttackGroupSize * 6 / 10
		if airFormThreshold < 2 {
			airFormThreshold = 2
		}
		c.rules = append(c.rules, &Rule{
			Name:         "form-air-attack",
			Priority:     airAttackPriority + SquadFormBonus,
			Category:     "squad-form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`(!SquadExists("air-attack") && len(UnassignedIdleAir()) >= %d) || (SquadNeedsReinforcement("air-attack") && len(UnassignedIdleAir()) >= 1)`, airFormThreshold),
			Action:       FormSquad("air-attack", "air", c.d.AirAttackGroupSize, "attack"),
			ActionSrc:    actionSrc("form-squad", "air-attack", "air", c.d.AirAttackGroupSize, "attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-attack",
			Priority:     airAttackPriority,
			Category:     "air-combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && BestAirTarget() != nil`, c.activationThreshold),
			Action:       SquadAirStrike("air-attack"),
			ActionSrc:    actionSrc("squad-air-strike", "air-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-reengage",
			Priority:     airAttackPriority - ReengageDiscount,
			Category:     "air-combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("air-attack") && SquadIdleCount("air-attack") > 0 && BestAirTarget() != nil`,
			Action:       SquadAirStrike("air-attack"),
			ActionSrc:    actionSrc("squad-air-strike", "air-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-air-attack-known-base",
			Priority:     airAttackPriority - KnownBaseDiscount,
			Category:     "air-combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, c.activationThreshold),
			Action:       SquadAttackKnownBase("air-attack", c.d.Aggression),
			ActionSrc:    actionSrc("squad-attack-known-base", "air-attack", c.d.Aggression),
		})
	}

	// --- Naval attack ---

	if c.d.NavalWeight > DoctrineEnabled {
		navalAttackPriority := lerp(200, 400, c.d.Aggression) - NavalDomainOffset

		// Same partial-roster fix as form-ground-attack (vimy-a8e).
		navalFormThreshold := c.d.NavalAttackGroupSize * 6 / 10
		if navalFormThreshold < 2 {
			navalFormThreshold = 2
		}
		c.rules = append(c.rules, &Rule{
			Name:         "form-naval-attack",
			Priority:     navalAttackPriority + SquadFormBonus,
			Category:     "squad-form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && ((!SquadExists("naval-attack") && len(UnassignedIdleNaval()) >= %d) || (SquadNeedsReinforcement("naval-attack") && len(UnassignedIdleNaval()) >= 1))`, navalFormThreshold),
			Action:       FormSquad("naval-attack", "naval", c.d.NavalAttackGroupSize, "attack"),
			ActionSrc:    actionSrc("form-squad", "naval-attack", "naval", c.d.NavalAttackGroupSize, "attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-attack",
			Priority:     navalAttackPriority,
			Category:     "naval-combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && SquadExists("naval-attack") && SquadReadyRatio("naval-attack") >= %.2f && NearestEnemy() != nil`, c.activationThreshold),
			Action:       SquadAttackMove("naval-attack"),
			ActionSrc:    actionSrc("squad-attack-move", "naval-attack"),
		})

		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-reengage",
			Priority:     navalAttackPriority - ReengageDiscount,
			Category:     "naval-combat",
			Exclusive:    false,
			ConditionSrc: `MapHasWater() && SquadExists("naval-attack") && SquadIdleCount("naval-attack") > 0 && NearestEnemy() != nil`,
			Action:       SquadAttackMove("naval-attack"),
			ActionSrc:    actionSrc("squad-attack-move", "naval-attack"),
		})

		// Fallback: attack last-known enemy base when fog hides all enemies.
		c.rules = append(c.rules, &Rule{
			Name:         "squad-naval-attack-known-base",
			Priority:     navalAttackPriority - KnownBaseDiscount,
			Category:     "naval-combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && SquadExists("naval-attack") && SquadReadyRatio("naval-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, c.activationThreshold),
			Action:       SquadAttackKnownBase("naval-attack", c.d.Aggression),
			ActionSrc:    actionSrc("squad-attack-known-base", "naval-attack", c.d.Aggression),
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
