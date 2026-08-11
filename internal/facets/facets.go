// Package facets owns the names and selection policy for stored thread facets.
// Protocol and application adapters should project this catalog rather than
// repeating facet names or default sets.
package facets

import (
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

// Facet names are stable corpus keys. Health-only facets are included here so
// every stored facet has one owner, even when it is not selectable hydration.
const (
	IssueComments    = "issue_comments"
	PRDetails        = "pr_details"
	PRReviews        = "pr_reviews"
	PRReviewComments = "pr_review_comments"
	PRChecks         = "pr_checks"
	PRReviewThreads  = "pr_review_threads"
	PRMergeState     = "pr_merge_state"
	PRMergeQueue     = "pr_merge_queue"
	PRClosingIssues  = "pr_closing_issues"
	PRFiles          = "pr_files"
	IssueTimeline    = "issue_timeline"
)

type definition struct {
	name   Name
	policy hydrationPolicy
}

type hydrationPolicy uint8

const (
	healthOnly hydrationPolicy = iota
	defaultForAllThreads
	defaultForPullRequests
	explicitForAllThreads
)

var catalog = [...]definition{
	{name: Name(IssueComments), policy: defaultForAllThreads},
	{name: Name(PRDetails), policy: defaultForPullRequests},
	{name: Name(PRReviews), policy: defaultForPullRequests},
	{name: Name(PRReviewComments), policy: defaultForPullRequests},
	{name: Name(PRChecks)},
	{name: Name(PRReviewThreads)},
	{name: Name(PRMergeState)},
	{name: Name(PRMergeQueue)},
	{name: Name(PRClosingIssues)},
	{name: Name(PRFiles)},
	{name: Name(IssueTimeline), policy: explicitForAllThreads},
}

// Name is a parsed catalog-backed facet name.
type Name string

// String returns the stable corpus and protocol key.
func (n Name) String() string { return string(n) }

// Selection is an immutable parsed hydration selection. Its zero value means
// the default facets for the resolved thread kind.
type Selection struct {
	requested []Name
}

// ParseSelection parses and deduplicates selectable hydration facets while
// preserving caller order. Applicability is checked after the thread kind is
// resolved.
func ParseSelection(values []string) (Selection, error) {
	requested := make([]Name, 0, len(values))
	seen := make(map[Name]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		definition, ok := definitionByName(value)
		if !ok || definition.policy == healthOnly {
			return Selection{}, fmt.Errorf("unknown facet %q", value)
		}
		if _, duplicate := seen[definition.name]; duplicate {
			continue
		}
		seen[definition.name] = struct{}{}
		requested = append(requested, definition.name)
	}
	return Selection{requested: requested}, nil
}

// Explicit reports whether the caller selected facets rather than requesting
// the defaults for each thread kind.
func (s Selection) Explicit() bool { return len(s.requested) > 0 }

// For returns the selected facets for one exact thread kind. An explicit facet
// that cannot apply to that kind is rejected.
func (s Selection) For(kind domain.ThreadKind) ([]Name, error) {
	if !s.Explicit() {
		return defaultNamesFor(kind), nil
	}
	result := make([]Name, 0, len(s.requested))
	for _, name := range s.requested {
		definition, _ := definitionByName(name.String())
		if !selectableFor(definition, kind) {
			return nil, fmt.Errorf("facet %q is not applicable to %s threads", name, kind)
		}
		result = append(result, name)
	}
	return result, nil
}

func definitionByName(name string) (definition, bool) {
	for _, facet := range catalog {
		if facet.name.String() == name {
			return facet, true
		}
	}
	return definition{}, false
}

func defaultNamesFor(kind domain.ThreadKind) []Name {
	result := make([]Name, 0, len(catalog))
	for _, facet := range catalog {
		if defaultFor(facet, kind) {
			result = append(result, facet.name)
		}
	}
	return result
}

func defaultFor(facet definition, kind domain.ThreadKind) bool {
	switch facet.policy {
	case defaultForAllThreads:
		return true
	case defaultForPullRequests:
		return kind == domain.PullRequestKind
	default:
		return false
	}
}

func selectableFor(facet definition, kind domain.ThreadKind) bool {
	return defaultFor(facet, kind) || facet.policy == explicitForAllThreads
}

// DefaultFor returns the default hydration facets for a thread kind.
func DefaultFor(kind domain.ThreadKind) []string {
	names := defaultNamesFor(kind)
	result := make([]string, len(names))
	for index, name := range names {
		result[index] = name.String()
	}
	return result
}

// SelectableFor returns facets accepted for explicit hydration of a thread
// kind. Timeline is intentionally explicit-only because it may be large.
func SelectableFor(kind domain.ThreadKind) []string {
	result := DefaultFor(kind)
	if len(result) == 0 {
		return nil
	}
	for _, facet := range catalog {
		if facet.policy == explicitForAllThreads {
			result = append(result, facet.name.String())
		}
	}
	return result
}

// SelectableNames returns the union used by protocol schemas. Per-thread
// applicability remains enforced by the application service.
func SelectableNames() []string {
	seen := make(map[string]struct{}, len(catalog))
	result := make([]string, 0, len(catalog))
	for _, kind := range []domain.ThreadKind{domain.IssueKind, domain.PullRequestKind} {
		for _, facet := range SelectableFor(kind) {
			if _, ok := seen[facet]; ok {
				continue
			}
			seen[facet] = struct{}{}
			result = append(result, facet)
		}
	}
	return result
}

// AllNames returns every stored facet key, including health-only projections.
func AllNames() []string {
	result := make([]string, 0, len(catalog))
	for _, facet := range catalog {
		result = append(result, facet.name.String())
	}
	return result
}
