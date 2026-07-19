// Package similarity owns exact, versioned thread similarity policies.
package similarity

// RuleVersion identifies the exact preparation and scoring semantics used for a result.
type RuleVersion string

const (
	// DuplicateV1 is the weighted duplicate-candidate scoring contract.
	DuplicateV1 RuleVersion = "duplicate-v1"
	// PrecedentV1 is the lexical Jaccard precedent scoring contract.
	PrecedentV1 RuleVersion = "precedent-v1"
)
