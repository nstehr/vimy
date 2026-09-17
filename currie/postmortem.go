package main

import (
	"context"
	"fmt"
	"io"
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

	// Rendered from the view the page builds, not from the raw report: the two
	// must agree about which rules count as dead, and they did not. The raw
	// blame lists rules the engine recorded acting — the sample is every 15th
	// evaluation, so a rule can act between samples — and calling those dead is
	// the mistake the reconcile step exists to prevent.
	v := buildWith(g.OurFaction, rep.Report, rep.Windows_, rep.Firings,
		g.DurationTicks, g.OurFaction, rep.Doctrines)
	writeNeverRan(w, v.NeverRan, top)
	writeBlame(w, v, rep, top)
	return nil
}

func writeNeverRan(w io.Writer, rows []preemptedRule, top int) {
	fmt.Fprintf(w, "\n  READY AND NEVER RAN (%d) — lost an exclusive category every time\n", len(rows))
	if len(rows) == 0 {
		fmt.Fprintln(w, "    none")
		return
	}
	for i, r := range rows {
		if i == top {
			fmt.Fprintf(w, "    ... and %d more\n", len(rows)-top)
			break
		}
		lost := "nothing that held"
		if len(r.LostTo) > 0 {
			lost = r.LostTo[0]
			if n := len(r.LostTo) - 1; n > 0 {
				lost = fmt.Sprintf("%s (+%d)", lost, n)
			}
		}
		fmt.Fprintf(w, "    %-28s %4d x (%2.0f%% of %3d seen) in %-16s to %s\n",
			r.Name, r.Preempted, r.Rate, r.Seen, r.Category+",", lost)
	}
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
	if sp.Overstated {
		fmt.Fprintf(w, "    !! the total is %.2fx everything earned, so it is WRONG. An act is a\n"+
			"       produce envelope, resent every %d ticks until the item appears, so one\n"+
			"       unit counts many times. Read the shares; do not read the credits.\n",
			sp.Inflation, 100)
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

// writeBlame lists the rules a single clause kept false, worst first.
//
// These are the ones with a gate to move, which is what separates them from
// the rules above: nothing was blocking those.
func writeBlame(w io.Writer, v view, rep *Replay, top int) {
	fmt.Fprintf(w, "\n  BLAME (%d states across %d doctrine windows) — never fired, one clause to blame\n",
		rep.Report.States, rep.Windows)
	if len(v.Dead) == 0 {
		fmt.Fprintln(w, "    none")
		return
	}
	for i, d := range v.Dead {
		if i == top {
			fmt.Fprintf(w, "    ... and %d more\n", len(v.Dead)-top)
			break
		}
		fmt.Fprintf(w, "    %-28s sole blocker %4d x (%.0f%%): %.46s\n",
			d.Name, d.Culprit.Sole, d.SolePct, strings.TrimSpace(d.Culprit.Source))
	}
}
