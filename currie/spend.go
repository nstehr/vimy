package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nstehr/vimy/vimy-core/store"
)

//go:embed rule-items.json
var ruleItems embed.FS

// What the money bought.
//
// The blame says which rules were stopped. It cannot say what the ones that
// ran cost, and that is the other half of the question: game 127 spent 67
// percent of a 189914-credit economy on one rule's output and the blame page
// showed nothing unusual, because nothing was blocking it.
//
// An estimate, and labelled as one everywhere it appears. A rule's act count
// is the number of times it sent a produce envelope, which is not the number
// of units that finished — cancelled and destroyed-in-queue production still
// counts, and rules that can build several things are priced at the
// representative one.
type Spend struct {
	Kinds []SpendKind
	Lines []SpendLine
	Total int
	// Against the engine's own Earned, so the ratio is honest about how much of
	// it was ever spent.
	Earned int
	// Items named by the rule table that the engine's yaml had no price for.
	// Counted rather than dropped: a total that silently omits a line is worse
	// than one that says it is short.
	Unpriced []string
}

// SpendKind is one bucket: armour, economy, infantry and so on.
type SpendKind struct {
	Kind    string
	Credits int
	Percent int
}

// SpendLine is one rule's contribution, dearest first.
type SpendLine struct {
	Rule    string
	Acts    int
	Price   int
	Credits int
}

// ruleItem is what a producing rule buys.
type ruleItem struct {
	Item string `json:"item"`
	Kind string `json:"kind"`
}

func loadRuleItems() (map[string]ruleItem, error) {
	b, err := ruleItems.ReadFile("rule-items.json")
	if err != nil {
		return nil, fmt.Errorf("rule-items.json: %w", err)
	}
	var doc struct {
		Rules map[string]ruleItem `json:"rules"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("rule-items.json: %w", err)
	}
	return doc.Rules, nil
}

var (
	yamlHeader = regexp.MustCompile(`^([A-Za-z0-9._]+):\s*$`)
	yamlCost   = regexp.MustCompile(`^\s+Cost:\s*(\d+)`)
)

// enginePrices reads unit and building costs from the engine's own rules.
//
// Parsed rather than transcribed: the whole value of pricing the spend is that
// the numbers are the mod's, and a copied table is wrong from the first
// balance change. Deliberately not a YAML parser — OpenRA's dialect has
// inheritance and overrides a real parser would have to model, and all that is
// wanted here is the first Cost under each top-level actor.
func enginePrices(dir string) (map[string]int, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s: no engine rules", dir)
	}
	sort.Strings(paths)
	prices := map[string]int{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue // a mod without this file prices less, not nothing
		}
		current := ""
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if m := yamlHeader.FindStringSubmatch(line); m != nil {
				// "E1.Husk" and "E1" are the same actor for pricing.
				current = strings.ToLower(strings.SplitN(m[1], ".", 2)[0])
				continue
			}
			if m := yamlCost.FindStringSubmatch(line); m != nil && current != "" {
				if _, seen := prices[current]; !seen {
					n, _ := strconv.Atoi(m[1])
					prices[current] = n
				}
			}
		}
		f.Close()
	}
	return prices, nil
}

// computeSpend prices what each rule did against the engine's own numbers.
//
// Acted rather than matched: a rule that matched and sent nothing bought
// nothing. Games predating the counter record -1, which is not zero and is
// skipped rather than read as "it did nothing".
func computeSpend(firings map[string]store.Firing, prices map[string]int, items map[string]ruleItem, earned int) Spend {
	s := Spend{Earned: earned}
	byKind := map[string]int{}
	missing := map[string]bool{}
	for name, f := range firings {
		if f.Acted <= 0 {
			continue
		}
		it, ok := items[name]
		if !ok {
			continue
		}
		price, ok := prices[it.Item]
		if !ok {
			missing[it.Item] = true
			continue
		}
		credits := price * f.Acted
		byKind[it.Kind] += credits
		s.Total += credits
		s.Lines = append(s.Lines, SpendLine{Rule: name, Acts: f.Acted, Price: price, Credits: credits})
	}
	for kind, credits := range byKind {
		pct := 0
		if s.Total > 0 {
			pct = credits * 100 / s.Total
		}
		s.Kinds = append(s.Kinds, SpendKind{Kind: kind, Credits: credits, Percent: pct})
	}
	sort.Slice(s.Kinds, func(i, j int) bool { return s.Kinds[i].Credits > s.Kinds[j].Credits })
	sort.Slice(s.Lines, func(i, j int) bool { return s.Lines[i].Credits > s.Lines[j].Credits })
	for item := range missing {
		s.Unpriced = append(s.Unpriced, item)
	}
	sort.Strings(s.Unpriced)
	return s
}
