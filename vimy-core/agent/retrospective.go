package agent

import (
	"context"
	"encoding/json"
	"log/slog"

	baml_client "github.com/nstehr/vimy/vimy-core/baml_client"
	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

// retrospectiveSnapshot is everything the review needs, taken under the
// strategist lock before Reset wipes the history.
type retrospectiveSnapshot struct {
	ourFaction      string
	opponentFaction string
	won             bool
	durationTicks   int
	mapWidth        int
	mapHeight       int
	history         []DoctrineRecord
	totalLosses     map[string]int
	losses          lossTracker
	army            armyValueTracker
	harvesters      harvesterTracker
	strikeBlocked   map[string]int
	rallyCount      int
	rallyMembers    int
	rallyIdle       int
	rallySpread     int
	transitSpread   string
	enemyUnits      string
	enemyBuildings  string
	engineStats     model.Player
	store           *store.Store
	// Where this game's states were written, for replay. Empty without
	// --export-states.
	exportPath string
	// onArchived seals the telemetry log once the game has an archive id.
	// Nil when streaming is off.
	onArchived func(gameID int64) error
	// The directive these doctrines were written from.
	directive string
}

// snapshotForReview must run before Reset, while the history still exists. Nil
// means there was nothing to review.
func (s *Strategist) snapshotForReview(won bool, exportPath string) *retrospectiveSnapshot {
	// Close the final doctrine window before copying, so the archive covers the
	// whole game rather than stopping at the last swap.
	var finalStats map[string]rules.RuleFiringStats
	if s.engine.TracingEnabled() {
		finalStats = s.engine.FlushFiringStats()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.history) == 0 || s.store == nil {
		return nil
	}
	if n := len(s.history); n > 0 && len(finalStats) > 0 {
		s.history[n-1].RuleStats = finalStats
	}

	rc, rm, ri, rs := s.engine.RallyShapeCounts()
	enemyUnitsSeen, enemyBuildingsSeen := s.engine.EnemySeenJSON()

	snap := &retrospectiveSnapshot{
		ourFaction:      s.faction,
		opponentFaction: s.opponentFaction,
		won:             won,
		history:         append([]DoctrineRecord(nil), s.history...),
		totalLosses:     make(map[string]int, len(s.totalLosses)),
		losses:          s.losses,
		army:            s.army,
		harvesters:      s.harvesters,
		strikeBlocked:   s.engine.StrikeBlockedCounts(),
		rallyCount:      rc,
		rallyMembers:    rm,
		rallyIdle:       ri,
		rallySpread:     rs,
		transitSpread:   s.engine.TransitSpreadJSON(),
		enemyUnits:      enemyUnitsSeen,
		enemyBuildings:  enemyBuildingsSeen,
		store:           s.store,
		exportPath:      exportPath,
		directive:       s.directive,
	}
	if snap.opponentFaction == "" {
		snap.opponentFaction = "unknown"
	}
	if s.latest != nil {
		snap.engineStats = s.latest.Player
		snap.durationTicks = s.latest.Tick
		snap.mapWidth = s.latest.MapWidth
		snap.mapHeight = s.latest.MapHeight
	}
	for k, v := range s.totalLosses {
		snap.totalLosses[k] = v
	}
	return snap
}

// runRetrospective launches the post-game review in a goroutine that outlives
// HandleGameEnd by design: the sidecar is long-running and the review is not
// latency-critical. A nil snapshot is a no-op.
func (s *Strategist) runRetrospective(ctx context.Context, snap *retrospectiveSnapshot) {
	if snap == nil {
		return
	}
	go doRetrospective(ctx, snap)
}

func doRetrospective(ctx context.Context, snap *retrospectiveSnapshot) {
	historyEntries := make([]types.DoctrineHistoryEntry, 0, len(snap.history))
	var events []types.GameEvent
	for _, rec := range snap.history {
		historyEntries = append(historyEntries, types.DoctrineHistoryEntry{
			Tick:     int64(rec.Tick),
			Doctrine: toBAMLDoctrine(rec.Doctrine),
		})
		for _, e := range rec.Events {
			events = append(events, types.GameEvent{
				Kind:   string(e.Kind),
				Tick:   int64(e.Tick),
				Detail: e.Detail,
			})
		}
	}

	combatStats := types.CombatStats{
		Infantry_lost: int64(snap.totalLosses["infantry"]),
		Vehicles_lost: int64(snap.totalLosses["vehicle"]),
		Aircraft_lost: int64(snap.totalLosses["aircraft"]),
		Naval_lost:    int64(snap.totalLosses["naval"]),

		// The engine's count, not the inference it disagrees with by 3x.
		Enemy_units_killed:        int64(snap.engineStats.UnitsKilled),
		Enemy_buildings_destroyed: int64(snap.engineStats.BuildingsKilled),
	}

	review, err := baml_client.ReviewGame(
		ctx,
		snap.ourFaction,
		snap.opponentFaction,
		snap.won,
		int64(snap.durationTicks),
		historyEntries,
		events,
		combatStats,
	)

	if err != nil {
		slog.Error("retrospective LLM call failed", "error", err)
	}

	archival := buildArchival(snap, review, err == nil)

	// Sealed whatever happens below: a log left open never ships its tail,
	// and Finish is idempotent so the successful call wins the race with this.
	if snap.onArchived != nil {
		defer func() {
			if err := snap.onArchived(0); err != nil {
				slog.Error("sealing the telemetry log failed", "error", err)
			}
		}()
	}

	gameID, archErr := snap.store.ArchiveGame(ctx, archival.gameCtx, archival.qualityTag, archival.reviewJSON, archival.doctrines)
	if archErr != nil {
		slog.Error("ArchiveGame failed", "error", archErr)
		return
	}
	if snap.onArchived != nil {
		if err := snap.onArchived(gameID); err != nil {
			slog.Error("sealing the telemetry log failed", "error", err)
		}
	}

	if len(archival.lessons) > 0 {
		if err := snap.store.ArchiveLessons(ctx, gameID, snap.ourFaction, snap.opponentFaction, archival.lessons); err != nil {
			slog.Error("ArchiveLessons failed", "error", err)
		}
	}

	if err == nil {
		slog.Info("retrospective complete",
			"quality", archival.qualityTag,
			"summary", review.Summary,
			"lessons", len(review.Lessons),
			"turning_points", len(review.Turning_points))
	}
}

// archivalPayload bundles everything that gets handed to the store after a
// retrospective. Extracted for testability.
type archivalPayload struct {
	gameCtx    store.GameContext
	qualityTag string
	reviewJSON string
	doctrines  []store.InputDoctrine
	lessons    []store.InputLesson
}

// buildArchival shapes a review into storage inputs; no I/O, no LLM, no locks.
// Without haveReview the review is ignored and the doctrines archive unrated.
func buildArchival(snap *retrospectiveSnapshot, review types.GameReview, haveReview bool) archivalPayload {
	out := archivalPayload{
		gameCtx: store.GameContext{
			OurFaction:      snap.ourFaction,
			OpponentFaction: snap.opponentFaction,
			MapWidth:        snap.mapWidth,
			MapHeight:       snap.mapHeight,
			DurationTicks:   snap.durationTicks,
			Won:             snap.won,
			ExportPath:      snap.exportPath,
			Directive:       snap.directive,

			InfantryLost: snap.totalLosses["infantry"],
			VehiclesLost: snap.totalLosses["vehicle"],

			InfantryLostForward: snap.losses.Forward["infantry"],
			VehiclesLostForward: snap.losses.Forward["vehicle"],

			EngineUnitsKilled:     snap.engineStats.UnitsKilled,
			EngineUnitsDead:       snap.engineStats.UnitsDead,
			EngineBuildingsKilled: snap.engineStats.BuildingsKilled,
			EngineBuildingsDead:   snap.engineStats.BuildingsDead,
			EngineKillsCost:       snap.engineStats.KillsCost,
			EngineDeathsCost:      snap.engineStats.DeathsCost,
			EngineArmyValue:       snap.engineStats.ArmyValue,
			EngineEarned:          snap.engineStats.Earned,

			HarvesterIdle:       snap.harvesters.Idle,
			HarvesterMining:     snap.harvesters.Mining,
			HarvesterTravelling: snap.harvesters.Travelling,
			HarvesterAtRefinery: snap.harvesters.AtRefinery,
			HarvesterHaulDist:   snap.harvesters.MeanHaulDistance(),

			StrikeBlockedUnclumped:   snap.strikeBlocked[rules.StrikeBlockedUnclumped],
			StrikeBlockedBlindAtBase: snap.strikeBlocked[rules.StrikeBlockedBlindAtBase],
			StrikeBlockedEnRoute:     snap.strikeBlocked[rules.StrikeBlockedNoTargetEnRoute],

			RallyCount:               snap.rallyCount,
			RallyMembersSum:          snap.rallyMembers,
			RallyIdleSum:             snap.rallyIdle,
			RallySpreadSum:           snap.rallySpread,
			TransitSpreadJSON:        snap.transitSpread,
			EnemyUnitsSeenJSON:       snap.enemyUnits,
			EnemyBuildingsSeenJSON:   snap.enemyBuildings,
			StrikeBlockedNotBuilding: snap.strikeBlocked[rules.StrikeBlockedNotBuilding],
			StrikeBlockedOutOfReach:  snap.strikeBlocked[rules.StrikeBlockedOutOfReach],

			OurArmyPeak:       snap.army.OursPeak,
			EnemyArmySeenPeak: snap.army.TheirsPeak,
			OurArmyMean:       armyMeanOurs(snap.army),
			EnemyArmySeenMean: armyMeanTheirs(snap.army),
		},
	}

	ratingByName := map[string]types.DoctrineRating{}
	if haveReview {
		out.qualityTag = review.Quality_tag
		if js, jErr := json.Marshal(review); jErr == nil {
			out.reviewJSON = string(js)
		} else {
			slog.Warn("marshal GameReview failed", "error", jErr)
		}
		for _, r := range review.Doctrine_ratings {
			ratingByName[r.Name] = r
		}
		for _, l := range review.Lessons {
			out.lessons = append(out.lessons, store.InputLesson{
				Trigger:    l.Trigger_text,
				Guidance:   l.Guidance_text,
				Confidence: l.Confidence,
			})
		}
	}

	for _, rec := range snap.history {
		dj, mErr := json.Marshal(rec.Doctrine)
		if mErr != nil {
			slog.Warn("marshal doctrine failed", "error", mErr)
			continue
		}
		entry := store.InputDoctrine{
			Tick:         rec.Tick,
			DoctrineJSON: string(dj),
		}
		if r, ok := ratingByName[rec.Doctrine.Name]; ok {
			entry.Rating = r.Rating
			entry.RatingReason = r.Reasoning
		}
		if len(rec.RuleSet) > 0 {
			if rs, err := json.Marshal(rec.RuleSet); err == nil {
				entry.RuleSetJSON = string(rs)
			}
		}
		for name, stats := range rec.RuleStats {
			entry.RuleFirings = append(entry.RuleFirings, store.InputRuleFiring{
				RuleName:  name,
				FireCount: stats.FireCount,
				ActCount:  stats.ActCount,
				FirstTick: stats.FirstTick,
				LastTick:  stats.LastTick,
			})
		}
		out.doctrines = append(out.doctrines, entry)
	}

	return out
}

// toBAMLDoctrine is the inverse of fromBAML, for putting a doctrine back into a
// prompt.
func toBAMLDoctrine(d rules.Doctrine) types.Doctrine {
	return types.Doctrine{
		Name:                           d.Name,
		Rationale:                      d.Rationale,
		Economy_priority:               d.EconomyPriority,
		Aggression:                     d.Aggression,
		Ground_defense_priority:        d.GroundDefensePriority,
		Air_defense_priority:           d.AirDefensePriority,
		Tech_priority:                  d.TechPriority,
		Infantry_weight:                d.InfantryWeight,
		Vehicle_weight:                 d.VehicleWeight,
		Air_weight:                     d.AirWeight,
		Naval_weight:                   d.NavalWeight,
		Ground_attack_group_size:       int64(d.GroundAttackGroupSize),
		Air_attack_group_size:          int64(d.AirAttackGroupSize),
		Naval_attack_group_size:        int64(d.NavalAttackGroupSize),
		Scout_priority:                 d.ScoutPriority,
		Specialized_infantry_weight:    d.SpecializedInfantryWeight,
		Superweapon_priority:           d.SuperweaponPriority,
		Capture_priority:               d.CapturePriority,
		Transport_assault:              d.TransportAssault,
		Preferred_infantry:             d.PreferredInfantry,
		Preferred_vehicle:              d.PreferredVehicle,
		Preferred_aircraft:             d.PreferredAircraft,
		Preferred_naval:                d.PreferredNaval,
		Ground_target_aa_priority:      d.GroundTargetAAPriority,
		Air_target_ground_def_priority: d.AirTargetGroundDefPriority,
	}
}

func armyMeanOurs(a armyValueTracker) int   { o, _ := a.Means(); return o }
func armyMeanTheirs(a armyValueTracker) int { _, t := a.Means(); return t }
