package rules

import "fmt"

// addMicroRules emits rules for unit micro-management: retreat, chase leash,
// engagement quality, focus fire, harvester flee, and recon scouting.
// Must be called after addCombatRules (uses c.attackPriority and
// c.activationThreshold).
func (c *doctrineCompiler) addMicroRules() {
	// --- Micro behaviors ---
	// Category "micro" is non-exclusive so all micro rules can co-fire on the same tick.

	// Retreat damaged combat units — always present since all doctrines have combat units.
	retreatThreshold := lerpf(0.50, 0.15, c.d.Aggression)
	retreatPriority := lerp(380, 450, 1.0-c.d.Aggression)
	c.rules = append(c.rules, &Rule{
		Name:         "retreat-damaged-units",
		Priority:     retreatPriority,
		Category:     "micro",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`len(DamagedCombatUnits(%.2f)) > 0`, retreatThreshold),
		Action:       RetreatDamagedUnits(retreatThreshold),
	})

	// Clear healed units from retreating set — runs every tick so healed
	// units return to the combat pool promptly.
	c.rules = append(c.rules, &Rule{
		Name:         "clear-healed-units",
		Priority:     500,
		Category:     "micro",
		Exclusive:    false,
		ConditionSrc: "HasRetreatingUnits()",
		Action:       ClearHealedUnits(retreatThreshold),
	})

	// Chase leash — recall overextended squad members that wandered off after kills.
	// Leash distance scales with aggression — aggressive doctrines let units roam further.
	leashPct := lerpf(0.25, 0.50, c.d.Aggression)
	for _, squadName := range []string{"ground-attack", "naval-attack"} {
		c.rules = append(c.rules, &Rule{
			Name:         fmt.Sprintf("recall-overextended-%s", squadName),
			Priority:     retreatPriority - 10,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("%s") && len(OverextendedSquadMembers("%s", %.2f)) > 0`, squadName, squadName, leashPct),
			Action:       RecallOverextended(squadName, leashPct),
		})
	}

	// Engagement quality — disengage when outnumbered locally.
	// Only active when aggression < 1.0 (pure aggression doctrines never retreat).
	if c.d.Aggression < 1.0 {
		// threatThreshold: 1.5 at aggression=0 (cautious), 3.0 at aggression=0.9 (aggressive)
		threatThreshold := lerpf(1.5, 3.0, c.d.Aggression)
		checkRadius := 0.10 // 10% of map diagonal
		for _, squadName := range []string{"ground-attack", "naval-attack"} {
			c.rules = append(c.rules, &Rule{
				Name:         fmt.Sprintf("squad-disengage-%s", squadName),
				Priority:     retreatPriority - 5,
				Category:     "micro",
				Exclusive:    false,
				ConditionSrc: fmt.Sprintf(`SquadExists("%s") && SquadAwayFromBase("%s", %.2f) && SquadThreatRatio("%s", %.2f) > %.2f`, squadName, squadName, checkRadius, squadName, checkRadius, threatThreshold),
				Action:       SquadDisengage(squadName),
			})
		}
	}

	// Focus fire on weakest visible enemy — aggressive doctrines only.
	if c.d.Aggression > DoctrineModerate {
		c.rules = append(c.rules, &Rule{
			Name:         "squad-focus-fire",
			Priority:     c.attackPriority + 1,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && BestGroundTarget() != nil`, c.activationThreshold),
			Action:       SquadFocusFire("ground-attack"),
		})
	}

	// Flee harvesters from danger — economy-focused doctrines.
	if c.d.EconomyPriority > DoctrineEnabled {
		dangerPct := lerpf(0.05, 0.15, c.d.EconomyPriority)
		fleePriority := lerp(150, 300, c.d.EconomyPriority)
		c.rules = append(c.rules, &Rule{
			Name:         "flee-harvesters",
			Priority:     fleePriority,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`EnemiesVisible() && len(HarvestersInDanger(%.2f)) > 0`, dangerPct),
			Action:       FleeHarvesters(dangerPct),
		})
	}

	// --- Recon ---
	// Rangers are dedicated scouts — sent immediately when idle. They use Move
	// (fast, no stopping to fight) and fan out to different waypoints.
	// Always patrol: maintains map vision and refreshes enemy intel. Without
	// this, rangers go permanently idle once intel is gathered and enemies
	// are visible — and they're excluded from combat rules.

	scoutPriority := lerp(250, 400, c.d.ScoutPriority)
	c.rules = append(c.rules, &Rule{
		Name:         "scout-with-scouts",
		Priority:     scoutPriority + SquadFormBonus,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: `len(IdleScouts()) > 0`,
		Action:       ActionScoutWithRangers,
	})

	// Generic fallback: scout with any idle ground units once army is large enough.
	// Gates on visibility, not intel — having historical intel doesn't mean we
	// know where enemies are now. Scouts resume patrolling whenever enemies
	// leave vision, preventing idle stalls at reached waypoints.
	c.rules = append(c.rules, &Rule{
		Name:         "scout-with-idle-units",
		Priority:     scoutPriority,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && len(UnassignedIdleGround()) >= %d`, c.d.GroundAttackGroupSize),
		Action:       ActionScoutWithIdleUnits,
	})
}
