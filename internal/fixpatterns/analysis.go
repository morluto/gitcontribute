// Package fixpatterns owns the deterministic semantic classification used by
// repository fix-pattern analysis. It has no storage, network, or protocol
// dependencies.
package fixpatterns

import (
	"slices"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

type Outcome string

const (
	Merged         Outcome = "merged"
	ClosedUnmerged Outcome = "closed_unmerged"
	Superseded     Outcome = "superseded"
	Open           Outcome = "open"
	Unknown        Outcome = "unknown"
)

type Relationship string

const (
	Closes              Relationship = "closes"
	References          Relationship = "references"
	ExplicitReplacement Relationship = "explicit_replacement"
	SimilarityOnly      Relationship = "similarity_only"
)

type ProofStyle string

const (
	RegressionTest ProofStyle = "regression_test"
	Reproduction   ProofStyle = "reproduction"
	Benchmark      ProofStyle = "benchmark"
	BeforeAfter    ProofStyle = "before_after"
	Screenshot     ProofStyle = "screenshot"
)

type PullRequest struct {
	State domain.ThreadState
	Body  string
	Merge domain.MergeStatus
}

type Classification struct {
	Outcome              Outcome
	Relationship         Relationship
	RelatedRepository    domain.RepoRef
	RelatedKind          domain.ThreadKind
	RelatedNumber        int
	RelationshipEvidence string
	ProofStyles          []ProofStyle
}

func Classify(repository domain.RepoRef, pullRequest PullRequest) Classification {
	classification := Classification{Relationship: SimilarityOnly, ProofStyles: DetectProofStyles(pullRequest.Body)}
	bestPriority := 0
	superseded := false
	for _, ref := range relatedwork.Extract(pullRequest.Body, repository) {
		if ref.Relation == relatedwork.RelationSupersededBy {
			superseded = true
		}
		priority := relatedwork.Priority(ref.Relation)
		if priority <= bestPriority {
			continue
		}
		bestPriority = priority
		classification.RelatedRepository = ref.Repo
		classification.RelatedKind = ref.Kind
		classification.RelatedNumber = ref.Number
		classification.RelationshipEvidence = ref.Evidence
		switch ref.Relation {
		case relatedwork.RelationClaimsToClose:
			classification.Relationship = Closes
		case relatedwork.RelationReplaces, relatedwork.RelationSupersededBy:
			classification.Relationship = ExplicitReplacement
		default:
			classification.Relationship = References
		}
	}
	classification.Outcome = classifyOutcome(pullRequest, superseded)
	return classification
}

func classifyOutcome(pullRequest PullRequest, superseded bool) Outcome {
	switch {
	case pullRequest.Merge.IsMerged():
		return Merged
	case pullRequest.State != domain.ClosedState:
		return Open
	case !pullRequest.Merge.Known():
		return Unknown
	case superseded:
		return Superseded
	default:
		return ClosedUnmerged
	}
}

func DetectProofStyles(body string) []ProofStyle {
	lower := strings.ToLower(body)
	var styles []ProofStyle
	for _, candidate := range []struct {
		style ProofStyle
		terms []string
	}{
		{RegressionTest, []string{"regression test", "unit test", "test coverage"}},
		{Reproduction, []string{"reproducer", "reproduction", "repro case"}},
		{Benchmark, []string{"benchmark", "throughput", "latency"}},
		{BeforeAfter, []string{"before and after", "before/after"}},
		{Screenshot, []string{"screenshot"}},
	} {
		if slices.ContainsFunc(candidate.terms, func(term string) bool { return strings.Contains(lower, term) }) {
			styles = append(styles, candidate.style)
		}
	}
	return styles
}
