package rules

import (
	"encoding/json"

	"github.com/nstehr/vimy/vimy-core/vimyc"
)

// RuleSetRecord is the durable input to an activation. Older exports omit it;
// readers must continue to distinguish reconstruction from recorded evidence.
type RuleSetRecord struct {
	Artifact       json.RawMessage `json:"artifact"`
	Doctrine       *Doctrine       `json:"doctrine,omitempty"`
	Sources        vimyc.Bundle    `json:"sources,omitempty"`
	SourceDigest   string          `json:"source_digest,omitempty"`
	CompilerDigest string          `json:"compiler_digest,omitempty"`
}

type Compilation struct {
	Rules  []*Rule
	Record RuleSetRecord
}

func artifactFor(rs []*Rule) json.RawMessage {
	source := make([]artifactRule, 0, len(rs))
	for _, r := range rs {
		name, err := actionName(r)
		if err != nil {
			return nil
		}
		source = append(source, artifactRule{Name: r.Name, Priority: r.Priority, Category: r.Category, Exclusive: r.Exclusive, Share: r.Share, Because: r.Because, Action: name, Condition: r.ConditionSrc, Source: r.Source})
	}
	raw, _ := json.Marshal(source)
	return raw
}
