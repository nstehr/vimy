package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nstehr/vimy/currie/ch"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
	"github.com/nstehr/vimy/vimy-core/vimyc"
)

type investigationResult struct {
	Insight *Insight
	Trace   []TraceStep
}

// analysisService owns expensive work and its identity; HTTP handlers only
// start jobs, wait for replay, and present immutable results.
type analysisService struct {
	rulesDir, bin  string
	engineRules    string
	store          *store.Store
	ch             *ch.Client
	insight        insighter
	cached         *cache
	runner         *jobRunner
	replays        jobSet[*Replay]
	insights       jobSet[*Insight]
	sweeps         jobSet[[]Sweep]
	investigations jobSet[investigationResult]
}

func (s *analysisService) startSweep(id int64, rep *Replay, knobs []string) {
	s.sweeps.start(s.runner, rep.key, 5*time.Minute, func(ctx context.Context) ([]Sweep, bool, error) {
		run := func(params map[string]float64, i int) (report, error) {
			r, _, err := blameStatesContext(ctx, params, rep.Windows_[i].Raw, s.rulesDir, s.bin, rep.Sources)
			return r, err
		}
		result, err := sweep(knobs, rep.Windows_, run)
		return result, true, err
	})
}
func (s *analysisService) startInsight(id int64, rep *Replay) {
	s.insights.start(s.runner, rep.key, 3*time.Minute, func(ctx context.Context) (*Insight, bool, error) {
		cache := s.cached.forRevision(rep.key.Revision)
		if prior := cache.read(id); prior != nil {
			return prior, true, nil
		}
		result, err := s.insight.Read(ctx, rep)
		if err == nil {
			cache.write(id, result)
		}
		return result, true, err
	})
}
func (s *analysisService) startInvestigation(id int64, rep *Replay) {
	s.investigations.start(s.runner, rep.key, 8*time.Minute, func(ctx context.Context) (investigationResult, bool, error) {
		cache := s.cached.forRevision(rep.key.Revision)
		if ins, trace := cache.readInvestigation(id); ins != nil {
			return investigationResult{ins, trace}, true, nil
		}
		iv := &investigator{ch: s.ch}
		ins, trace, settled, err := iv.Investigate(ctx, rep)
		if err == nil && settled {
			cache.writeInvestigation(id, ins, trace)
		}
		return investigationResult{ins, trace}, settled, err
	})
}

func (s *analysisService) replay(ctx context.Context, id int64) (*Replay, error) {
	games, err := s.store.ReplayableGames(ctx)
	if err != nil {
		return nil, err
	}
	var game *store.ReplayableGame
	for i := range games {
		if games[i].ID == id {
			game = &games[i]
			break
		}
	}
	if game == nil {
		return nil, fmt.Errorf("game %d has no export recorded; link an export first", id)
	}
	sources, err := vimyc.Sources(s.rulesDir)
	if err != nil {
		return nil, err
	}
	bundle, err := vimyc.ReadBundle(sources)
	if err != nil {
		return nil, err
	}
	compiler, err := vimyc.BinaryDigest(s.bin)
	if err != nil {
		return nil, err
	}
	raw, err := rules.ReadExport(expand(game.ExportPath))
	if err != nil {
		return nil, err
	}
	doctrines, err := s.store.DoctrinesForGame(ctx, id)
	if err != nil {
		return nil, err
	}
	firings, err := s.store.FiringsForGame(ctx, id)
	if err != nil {
		return nil, err
	}
	// Include all replay inputs, not just the game id. A relink, rule edit,
	// compiler rebuild or changed archive must miss every derived cache.
	payload, err := json.Marshal(struct {
		Version   int
		Sources   vimyc.Bundle
		Compiler  string
		Game      store.ReplayableGame
		Doctrines []store.DoctrineWindow
		Firings   map[string]store.Firing
	}{analysisVersion, bundle, compiler, *game, doctrines, firings})
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(payload)
	h.Write(raw)
	key := analysisKey{Game: id, Revision: hex.EncodeToString(h.Sum(nil))}
	j := s.replays.start(s.runner, key, 5*time.Minute, func(jobCtx context.Context) (*Replay, bool, error) {
		paths, cleanup, err := bundle.Materialize()
		if err != nil {
			return nil, false, err
		}
		defer cleanup()
		// The reader receives the captured export and archive rows as well as the
		// frozen sources, so the cache identity describes the actual computation.
		rep, err := replayInputs(jobCtx, *game, raw, doctrines, firings, paths, s.bin)
		if err != nil {
			return nil, false, err
		}
		after, err := vimyc.BinaryDigest(s.bin)
		if err != nil {
			return nil, false, err
		}
		if after != compiler {
			return nil, false, fmt.Errorf("compiler changed during replay; retry")
		}
		rep.key, rep.Sources = key, bundle
		return rep, true, nil
	})
	return j.wait(ctx)
}

func (s *analysisService) Close() error { s.runner.Close(); return s.cached.Close() }

func (s *analysisService) gameView(ctx context.Context, rep *Replay) view {
	v := buildWith(fmt.Sprintf("game %d · %s vs %s · %s", rep.Game.ID, rep.Game.OurFaction,
		rep.Game.OpponentFaction, outcome(rep.Game.Won)), rep.Report, rep.Windows_, rep.Firings, rep.Game.DurationTicks, rep.Game.OurFaction, rep.Doctrines)
	v.Windows = rep.Windows
	v.Orphaned = rep.Orphaned
	v.Approximate = rep.Approximate
	v.Warnings = rep.Warnings
	s.addOutcome(ctx, &v, rep)
	// Synchronous, unlike the reading and the sweep: these are three aggregate
	// queries against a columnar store and they answer in milliseconds, so the
	// cost of a poll endpoint would exceed the cost of the wait. The client's
	// own timeout is what keeps a server that is up but grinding from holding
	// the page.
	v.Stream = loadStream(ctx, s.ch, rep.Game.ID, 16)
	// The stream knows WHICH rules never fired; the replay knows WHY. Joined
	// here because this is the first place both exist.
	explainSilent(v.Stream, v.Dead, v.NeverRan)

	return v
}

// addOutcome attaches what the engine recorded and what the money bought.
//
// Best-effort: a game played before the statistics migration has none of this,
// and a missing ledger is not a reason to withhold the blame. Every absence is
// a skipped section rather than an error, and never a zero — "destroyed no
// buildings" and "was not counting buildings" are different findings.
func (s *analysisService) addOutcome(ctx context.Context, v *view, rep *Replay) {
	o, err := s.store.GameOutcome(ctx, rep.Game.ID)
	if err != nil {
		slog.Warn("no outcome", "game", rep.Game.ID, "error", err)
		return
	}
	v.Outcome = &o
	if o.HasTrade {
		v.TradeRatio = fmt.Sprintf("%.2f", o.TradeRatio())
		v.TradeWon = o.TradeRatio() < 1
	}
	if o.HasArmy && o.OurArmyPeak > 0 {
		v.ArmyRatio = fmt.Sprintf("%.1f", float64(o.EnemyArmySeenPeak)/float64(o.OurArmyPeak))
	}

	items, err := loadRuleItems()
	if err != nil {
		slog.Warn("no rule items", "error", err)
		return
	}
	prices, err := enginePrices(s.engineRules)
	if err != nil {
		// Currie runs from anywhere; without the engine checkout there are no
		// prices, and an unpriced spend is worse than none.
		slog.Warn("no engine prices", "dir", s.engineRules, "error", err)
		return
	}
	sp := computeSpend(rep.Firings, prices, items, o.Earned)
	if sp.Total > 0 {
		v.Spend = &sp
	}
}
