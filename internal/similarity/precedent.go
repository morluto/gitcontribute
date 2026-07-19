package similarity

import (
	"strings"
	"unicode"
)

// PrecedentRule is the exact precedent-v1 lexical Jaccard policy.
type PrecedentRule struct{}

// DefaultPrecedentRule returns the supported precedent-v1 scoring policy.
func DefaultPrecedentRule() PrecedentRule { return PrecedentRule{} }

// Version identifies the exact precedent scoring semantics.
func (PrecedentRule) Version() RuleVersion { return PrecedentV1 }

// PreparedLexical is an immutable precedent-v1 token set.
type PreparedLexical struct {
	tokens map[string]struct{}
}

// Prepare lowercases text and retains unique alphanumeric tokens of at least three bytes.
func (PrecedentRule) Prepare(text string) PreparedLexical {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) >= 3 {
			tokens[field] = struct{}{}
		}
	}
	return PreparedLexical{tokens: tokens}
}

// Compare returns exact Jaccard similarity under precedent-v1.
func (PrecedentRule) Compare(a, b PreparedLexical) float64 {
	if len(a.tokens) == 0 || len(b.tokens) == 0 {
		return 0
	}
	intersection := 0
	for token := range a.tokens {
		if _, exists := b.tokens[token]; exists {
			intersection++
		}
	}
	return float64(intersection) / float64(len(a.tokens)+len(b.tokens)-intersection)
}
