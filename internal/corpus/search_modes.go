package corpus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

const (
	// DefaultSearchPageSize is used when a boundary omits its page size.
	DefaultSearchPageSize = 20
	// MaximumSearchPageSize bounds one local FTS query.
	MaximumSearchPageSize = 100
)

// SearchPage is a parsed bounded page request. Its private fields prevent an
// invalid limit from reaching a query, while the zero value is the default
// first page.
type SearchPage struct {
	limit  int
	cursor string
}

// ParseSearchPage parses a boundary limit and opaque cursor.
func ParseSearchPage(limit int, cursor string) (SearchPage, error) {
	if limit == 0 {
		limit = DefaultSearchPageSize
	}
	if limit < 1 {
		return SearchPage{}, errors.New("search limit must be positive")
	}
	if limit > MaximumSearchPageSize {
		return SearchPage{}, errors.New("search limit cannot exceed 100")
	}
	return SearchPage{limit: limit, cursor: cursor}, nil
}

// MaximumSearchPage returns the largest supported first page.
func MaximumSearchPage() SearchPage { return SearchPage{limit: MaximumSearchPageSize} }

// Limit returns the bounded page size.
func (p SearchPage) Limit() int {
	if p.limit == 0 {
		return DefaultSearchPageSize
	}
	return p.limit
}

// Cursor returns the opaque continuation cursor, or empty for the first page.
func (p SearchPage) Cursor() string { return p.cursor }

// WithCursor returns the same bounded page size at the supplied continuation.
func (p SearchPage) WithCursor(cursor string) SearchPage {
	p.cursor = cursor
	return p
}

// ThreadRepositoryScope binds the stored repository row used by SQL to the
// parsed repository identity embedded in cursor scope. Its zero value searches
// every repository.
type ThreadRepositoryScope struct {
	ref domain.RepoRef
	id  int64
}

// NewThreadRepositoryScope constructs an exact stored-repository restriction.
func NewThreadRepositoryScope(ref domain.RepoRef, id int64) (ThreadRepositoryScope, error) {
	if !ref.IsValid() {
		return ThreadRepositoryScope{}, errors.New("repository reference is not parsed")
	}
	if id <= 0 {
		return ThreadRepositoryScope{}, fmt.Errorf("repository id must be positive, got %d", id)
	}
	return ThreadRepositoryScope{ref: ref, id: id}, nil
}

// AllThreadRepositories returns an unrestricted repository scope.
func AllThreadRepositories() ThreadRepositoryScope { return ThreadRepositoryScope{} }

// IsScoped reports whether one exact stored repository is selected.
func (s ThreadRepositoryScope) IsScoped() bool { return s.id != 0 }

// ID returns the selected stored repository row, or zero for every repository.
func (s ThreadRepositoryScope) ID() int64 { return s.id }

// Repository returns the selected parsed identity, or the zero value for every repository.
func (s ThreadRepositoryScope) Repository() domain.RepoRef { return s.ref }

// String returns the cursor-scope spelling, or empty for every repository.
func (s ThreadRepositoryScope) String() string { return s.ref.String() }

// SearchOrder is a parsed ordering for the corpus FTS indexes. Its private
// representation leaves relevance as the valid zero value.
type SearchOrder struct {
	updated bool
}

// ParseSearchOrder parses the only supported corpus search orderings.
func ParseSearchOrder(value string) (SearchOrder, error) {
	switch strings.TrimSpace(value) {
	case "", "relevance":
		return SearchOrder{}, nil
	case "updated":
		return SearchOrder{updated: true}, nil
	default:
		return SearchOrder{}, errors.New("search sort must be relevance or updated")
	}
}

// RelevanceSearchOrder returns the default weighted-FTS ordering.
func RelevanceSearchOrder() SearchOrder { return SearchOrder{} }

// UpdatedSearchOrder returns newest-source-update ordering.
func UpdatedSearchOrder() SearchOrder { return SearchOrder{updated: true} }

// IsUpdated reports whether newest-source-update ordering was selected.
func (o SearchOrder) IsUpdated() bool { return o.updated }

// String returns the boundary spelling of the parsed order.
func (o SearchOrder) String() string {
	if o.updated {
		return "updated"
	}
	return "relevance"
}

// TermMatch is a parsed FTS term-combination rule. Its private representation
// leaves all-term matching as the valid zero value.
type TermMatch struct {
	any bool
}

// ParseTermMatch parses the only supported term-combination rules.
func ParseTermMatch(value string) (TermMatch, error) {
	switch strings.TrimSpace(value) {
	case "", "all":
		return TermMatch{}, nil
	case "any":
		return TermMatch{any: true}, nil
	default:
		return TermMatch{}, errors.New("search match mode must be all or any")
	}
}

// MatchAllTerms returns the default conjunctive term rule.
func MatchAllTerms() TermMatch { return TermMatch{} }

// MatchAnyTerm returns the disjunctive term rule.
func MatchAnyTerm() TermMatch { return TermMatch{any: true} }

// IsAny reports whether at least one search term may match.
func (m TermMatch) IsAny() bool { return m.any }

// String returns the boundary spelling of the parsed term rule.
func (m TermMatch) String() string {
	if m.any {
		return "any"
	}
	return "all"
}

// ThreadKindFilter is an optional parsed issue-or-pull-request restriction.
// Its zero value includes both kinds.
type ThreadKindFilter struct {
	kind domain.ThreadKind
}

// ParseThreadKindFilter parses a canonical optional thread kind.
func ParseThreadKindFilter(value string) (ThreadKindFilter, error) {
	if strings.TrimSpace(value) == "" {
		return ThreadKindFilter{}, nil
	}
	kind, err := domain.ParseThreadKind(value)
	if err != nil {
		return ThreadKindFilter{}, err
	}
	return ThreadKindFilter{kind: kind}, nil
}

// AnyThreadKind includes issues and pull requests.
func AnyThreadKind() ThreadKindFilter { return ThreadKindFilter{} }

// IssueThreadKind restricts a search to issues.
func IssueThreadKind() ThreadKindFilter { return ThreadKindFilter{kind: domain.IssueKind} }

// PullRequestThreadKind restricts a search to pull requests.
func PullRequestThreadKind() ThreadKindFilter {
	return ThreadKindFilter{kind: domain.PullRequestKind}
}

// IsAny reports whether both thread kinds are included.
func (f ThreadKindFilter) IsAny() bool { return f.kind == "" }

// String returns the canonical stored thread-kind spelling, or empty for both.
func (f ThreadKindFilter) String() string { return string(f.kind) }

// ThreadStateFilter is an optional parsed open-or-closed restriction. Its zero
// value includes both states.
type ThreadStateFilter struct {
	state domain.ThreadState
}

// ParseThreadStateFilter parses a canonical optional thread state.
func ParseThreadStateFilter(value string) (ThreadStateFilter, error) {
	if value = strings.TrimSpace(value); value == "" || value == "all" {
		return ThreadStateFilter{}, nil
	}
	state, err := domain.ParseThreadState(value)
	if err != nil {
		return ThreadStateFilter{}, err
	}
	return ThreadStateFilter{state: state}, nil
}

// AnyThreadState includes open and closed threads.
func AnyThreadState() ThreadStateFilter { return ThreadStateFilter{} }

// OpenThreadState restricts a search to open threads.
func OpenThreadState() ThreadStateFilter { return ThreadStateFilter{state: domain.OpenState} }

// ClosedThreadState restricts a search to closed threads.
func ClosedThreadState() ThreadStateFilter { return ThreadStateFilter{state: domain.ClosedState} }

// IsAny reports whether both thread states are included.
func (f ThreadStateFilter) IsAny() bool { return f.state == "" }

// IsOpen reports whether the filter selects only open threads.
func (f ThreadStateFilter) IsOpen() bool { return f.state == domain.OpenState }

// String returns the canonical stored thread-state spelling, or empty for both.
func (f ThreadStateFilter) String() string { return string(f.state) }

// ThreadStateReason is an optional parsed GitHub close reason. Its zero value
// includes every reason.
type ThreadStateReason struct {
	value uint8
}

// ParseThreadStateReason parses the supported close reasons.
func ParseThreadStateReason(value string) (ThreadStateReason, error) {
	switch strings.TrimSpace(value) {
	case "":
		return ThreadStateReason{}, nil
	case "completed":
		return ThreadStateReason{value: 1}, nil
	case "not_planned":
		return ThreadStateReason{value: 2}, nil
	default:
		return ThreadStateReason{}, errors.New("search state reason must be completed or not_planned")
	}
}

// IsAny reports whether every close reason is included.
func (r ThreadStateReason) IsAny() bool { return r.value == 0 }

// String returns the canonical stored close reason, or empty for every reason.
func (r ThreadStateReason) String() string {
	switch r.value {
	case 1:
		return "completed"
	case 2:
		return "not_planned"
	default:
		return ""
	}
}

// MergeFilter selects merged, unmerged, unknown, or unrestricted outcomes.
// Its zero value is unrestricted.
type MergeFilter struct {
	value uint8
}

// ParseMergeFilter parses boolean and domain spellings used by search
// boundaries. Unknown is distinct from unrestricted so callers never need a
// second "known" flag.
func ParseMergeFilter(value string) (MergeFilter, error) {
	switch strings.TrimSpace(value) {
	case "", "any":
		return MergeFilter{}, nil
	case "true", "merged":
		return MergeFilter{value: 1}, nil
	case "false", "unmerged":
		return MergeFilter{value: 2}, nil
	case "unknown":
		return MergeFilter{value: 3}, nil
	default:
		return MergeFilter{}, errors.New("merge filter must be merged, unmerged, unknown, or any")
	}
}

// MergeFilterFromPointer parses the optional boolean boundary representation.
func MergeFilterFromPointer(value *bool) MergeFilter {
	if value == nil {
		return MergeFilter{}
	}
	if *value {
		return MergeFilter{value: 1}
	}
	return MergeFilter{value: 2}
}

// AnyMergeState includes known merged, known unmerged, and unknown results.
func AnyMergeState() MergeFilter { return MergeFilter{} }

// IsAny reports whether merge state is unrestricted.
func (f MergeFilter) IsAny() bool { return f.value == 0 }

// IsMerged reports whether only known merged pull requests are selected.
func (f MergeFilter) IsMerged() bool { return f.value == 1 }

// IsUnmerged reports whether only known unmerged pull requests are selected.
func (f MergeFilter) IsUnmerged() bool { return f.value == 2 }

// IsUnknown reports whether only pull requests without an observed merge
// state are selected.
func (f MergeFilter) IsUnknown() bool { return f.value == 3 }

// String returns the stable cursor-key spelling of the filter.
func (f MergeFilter) String() string {
	switch f.value {
	case 1:
		return "merged"
	case 2:
		return "unmerged"
	case 3:
		return "unknown"
	default:
		return "any"
	}
}

// BooleanString returns the legacy feedback-boundary spelling.
func (f MergeFilter) BooleanString() string {
	switch f.value {
	case 1:
		return "true"
	case 2:
		return "false"
	case 3:
		return "unknown"
	default:
		return "any"
	}
}
