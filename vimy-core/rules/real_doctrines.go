package rules

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Doctrines the LLM actually produced, sampled from the archive and embedded so
// tests need no database.
//
// Random doctrines are a poor stand-in: real ones cluster hard — naval_weight
// averages 0.011 against economy_priority's 0.761, and nothing reaches 1.0 — so
// uniform sampling tests shapes that never occur and under-tests the dominant
// ones.
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
