package rules

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Doctrines the LLM actually produced, sampled from 4,876 archived across 64
// games. Embedded so tests need no database.
//
// Randomly generated doctrines are a poor stand-in: real ones are strongly
// clustered, and sampling uniformly both tests rule shapes that never occur and
// under-tests the ones that dominate. Measured across the full 4,876 —
// `naval_weight` averages 0.011, `economy_priority` 0.761, and nothing ever
// reaches 1.0.
//
// Regenerate with the query in vimyc/docs/corpus.md.
//
//go:embed real_doctrines.json
var realDoctrinesJSON []byte

// RealDoctrines returns the sampled doctrines, for tests that want a corpus
// shaped like real play rather than like a random number generator.
func RealDoctrines() ([]Doctrine, error) {
	var out []Doctrine
	if err := json.Unmarshal(realDoctrinesJSON, &out); err != nil {
		return nil, fmt.Errorf("real_doctrines.json: %w", err)
	}
	for i := range out {
		out[i].Validate()
	}
	return out, nil
}
