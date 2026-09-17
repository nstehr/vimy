package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/store"
)

// The same four analyses as the report page, written to a terminal.
//
// Not a second implementation: it replays through replayGame and prices
// through computeSpend, so the two views cannot disagree. What it exists for
// is the moment after a game ends, when the question is "what happened" and
// starting a server to find out is friction enough that the question goes
// unasked. That is not hypothetical — `vimyc --blame` sat unused for most of a
// session and its findings were hand-rolled four separate times instead.
//
// It replaced a Python script that took the same shape and got the blame
// wrong: the script blamed every sampled state against ONE of the game's
// doctrines, and game 127 ran 121 of them. Pairing each state to the doctrine
// that was actually running is the whole point of the replay, so a second tool
// that skipped it was not a convenience, it was a wrong answer that looked
// like the right one.
func (s *server) postmortem(ctx context.Context, w io.Writer, id int64, top int) error {
	rep, err := s.replay(ctx, id)
	if err != nil {
		return err
	}
	g := rep.Game

	result := "lost"
	if g.Won {
		result = "WON"
	}
	fmt.Fprintf(w, "GAME %d: %d ticks, %s, %s vs %s\n",
		g.ID, g.DurationTicks, result, g.OurFaction, g.OpponentFaction)

	o, oerr := s.store.GameOutcome(ctx, id)
	if oerr == nil {
		writeLedger(w, o)
	}
	s.writeSpend(w, rep, o)
	writeWarnings(w, rep.Warnings)
	writeBlame(w, rep, top)
	return nil
}

func writeLedger(w io.Writer, o store.Outcome) {
	if o.HasTrade {
		fmt.Fprintf(w, "  trade      destroyed %d credits, lost %d  = 1:%.2f\n",
			o.KillsCost, o.DeathsCost, o.TradeRatio())
	}
	fmt.Fprintf(w, "  buildings  destroyed %d\n", o.BuildingsKilled)
	if o.HasArmy && o.OurArmyPeak > 0 {
		fmt.Fprintf(w, "  army peak  ours %d, theirs %d (seen — a floor)  = %.1fx\n",
			o.OurArmyPeak, o.EnemyArmySeenPeak,
			float64(o.EnemyArmySeenPeak)/float64(o.OurArmyPeak))
	}
	if o.HasLosses {
		fmt.Fprintf(w, "  losses     %d infantry, %d vehicles (%d forward, %d at home)\n",
			o.InfantryLost, o.VehiclesLost, o.VehiclesLostForward, o.VehiclesLostAtHome())
	}
}

func (s *server) writeSpend(w io.Writer, rep *Replay, o store.Outcome) {
	items, err := loadRuleItems()
	if err != nil {
		return
	}
	prices, err := enginePrices(s.engineRules)
	if err != nil {
		fmt.Fprintf(w, "\n  no engine rules at %s; skipping spend\n", s.engineRules)
		return
	}
	sp := computeSpend(rep.Firings, prices, items, o.Earned)
	if sp.Total == 0 {
		return
	}
	fmt.Fprintf(w, "\n  SPEND (estimated: rule acts x engine price)")
	if sp.Earned > 0 {
		fmt.Fprintf(w, ", against %d earned", sp.Earned)
	}
	fmt.Fprintln(w)
	for _, k := range sp.Kinds {
		fmt.Fprintf(w, "    %-12s%8d  %3d%%\n", k.Kind, k.Credits, k.Percent)
	}
	fmt.Fprintf(w, "    %-12s%8d\n", "TOTAL", sp.Total)
	fmt.Fprintln(w, "    largest lines:")
	for i, l := range sp.Lines {
		if i == 6 {
			break
		}
		fmt.Fprintf(w, "      %-30s%4d x %-5d = %8d\n", l.Rule, l.Acts, l.Price, l.Credits)
	}
	if len(sp.Unpriced) > 0 {
		fmt.Fprintf(w, "    unpriced, omitted from the total: %s\n", strings.Join(sp.Unpriced, " "))
	}
}

func writeWarnings(w io.Writer, warnings []Warning) {
	fmt.Fprintf(w, "\n  COMPILER (%d warning%s)\n", len(warnings), plural(len(warnings)))
	if len(warnings) == 0 {
		fmt.Fprintln(w, "    none")
		return
	}
	for _, x := range warnings {
		fmt.Fprintf(w, "    %s\n      in %d of %d doctrine windows\n", x.Text, x.Windows, x.Total)
	}
}

func writeBlame(w io.Writer, rep *Replay, top int) {
	fmt.Fprintf(w, "\n  BLAME (%d states across %d doctrine windows) — rules that never held\n",
		rep.Report.States, rep.Windows)

	type row struct {
		rule      string
		sole      int
		preempted int
		source    string
	}
	var rows []row
	for _, r := range rep.Report.Rules {
		if r.Held > 0 || r.Seen == 0 {
			continue
		}
		worst := row{rule: r.Rule, preempted: r.Preempted}
		for _, c := range r.Clauses {
			if c.Sole > worst.sole {
				worst.sole, worst.source = c.Sole, strings.TrimSpace(c.Source)
			}
		}
		rows = append(rows, worst)
	}
	// Preemption first: a rule blocked by its own clause is a gate to move, but
	// a rule that keeps losing its category never enters the contest at all,
	// and no amount of loosening its conditions will help.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].preempted != rows[j].preempted {
			return rows[i].preempted > rows[j].preempted
		}
		return rows[i].sole > rows[j].sole
	})
	for i, r := range rows {
		if i == top {
			fmt.Fprintf(w, "    ... and %d more\n", len(rows)-top)
			break
		}
		switch {
		case r.preempted > 0:
			fmt.Fprintf(w, "    %-30s preempted %d x; lost its category\n", r.rule, r.preempted)
		case r.sole > 0:
			fmt.Fprintf(w, "    %-30s sole blocker %d x: %.52s\n", r.rule, r.sole, r.source)
		default:
			fmt.Fprintf(w, "    %-30s never held; no single culprit\n", r.rule)
		}
	}
}
