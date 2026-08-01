// Package repositorycontext owns the bounded repository-level GitHub reads
// shared by application planning and public adapters.
package repositorycontext

import "slices"

// GuidancePaths is the complete fixed set of contribution and AI policy files
// probed by a repository-context refresh.
var guidancePaths = []string{
	".github/CONTRIBUTING.md",
	"CONTRIBUTING.md",
	"docs/CONTRIBUTING.md",
	".github/AI_POLICY.md",
	".github/AI-CONTRIBUTION-POLICY.md",
	".github/GENERATIVE_AI_POLICY.md",
	"AI_POLICY.md",
	"AI.md",
}

// GuidancePaths returns a copy of the fixed path policy.
func GuidancePaths() []string {
	return slices.Clone(guidancePaths)
}

// RequestCost is one metadata request, one ref-resolution request, plus every
// fixed guidance-path probe.
func RequestCost() int {
	return 2 + len(guidancePaths)
}
