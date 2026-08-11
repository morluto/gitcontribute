package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

type searchKind uint8

const (
	searchThreads searchKind = iota
	searchIssues
	searchPullRequests
	searchRepositories
	searchCodeDocuments
	searchAll
)

func parseSearchKind(value string) (searchKind, error) {
	switch strings.TrimSpace(value) {
	case "", "threads":
		return searchThreads, nil
	case "issue", "issues":
		return searchIssues, nil
	case "pr", "prs", "pull_request":
		return searchPullRequests, nil
	case "repos":
		return searchRepositories, nil
	case "code":
		return searchCodeDocuments, nil
	case "all":
		return searchAll, nil
	default:
		return 0, fmt.Errorf("unsupported search kind %q", value)
	}
}

func (k searchKind) String() string {
	switch k {
	case searchThreads:
		return "threads"
	case searchIssues:
		return "issues"
	case searchPullRequests:
		return "prs"
	case searchRepositories:
		return "repos"
	case searchCodeDocuments:
		return "code"
	case searchAll:
		return "all"
	default:
		return ""
	}
}

func (k searchKind) corpusThreadKind() corpus.ThreadKindFilter {
	switch k {
	case searchIssues:
		return corpus.IssueThreadKind()
	case searchPullRequests:
		return corpus.PullRequestThreadKind()
	default:
		return corpus.AnyThreadKind()
	}
}

type threadSearchCriteria struct {
	kind          searchKind
	state         corpus.ThreadStateFilter
	stateReason   corpus.ThreadStateReason
	merge         corpus.MergeFilter
	author        string
	association   string
	assignee      string
	labels        []string
	updatedAfter  time.Time
	updatedBefore time.Time
	order         corpus.SearchOrder
	match         corpus.TermMatch
}

func (c threadSearchCriteria) hasMetadataFilters() bool {
	return !c.state.IsAny() || !c.stateReason.IsAny() || !c.merge.IsAny() ||
		c.author != "" || c.association != "" || c.assignee != "" || len(c.labels) > 0 ||
		!c.updatedAfter.IsZero() || !c.updatedBefore.IsZero()
}

type searchRead struct {
	query         string
	page          corpus.SearchPage
	snapshotToken string
}

type parsedSearchRequest interface {
	read() searchRead
	kind() searchKind
	repository() domain.RepoRef
	isParsedSearchRequest()
}

type repositorySearchRequest struct {
	searchRead
	repo  domain.RepoRef
	order corpus.SearchOrder
}

func (r repositorySearchRequest) read() searchRead           { return r.searchRead }
func (repositorySearchRequest) kind() searchKind             { return searchRepositories }
func (r repositorySearchRequest) repository() domain.RepoRef { return r.repo }
func (repositorySearchRequest) isParsedSearchRequest()       {}

type threadSearchRequest struct {
	searchRead
	repo     domain.RepoRef
	criteria threadSearchCriteria
}

func (r threadSearchRequest) read() searchRead           { return r.searchRead }
func (r threadSearchRequest) kind() searchKind           { return r.criteria.kind }
func (r threadSearchRequest) repository() domain.RepoRef { return r.repo }
func (threadSearchRequest) isParsedSearchRequest()       {}

type codeSearchRequest struct {
	searchRead
	repo domain.RepoRef
}

func (r codeSearchRequest) read() searchRead           { return r.searchRead }
func (codeSearchRequest) kind() searchKind             { return searchCodeDocuments }
func (r codeSearchRequest) repository() domain.RepoRef { return r.repo }
func (codeSearchRequest) isParsedSearchRequest()       {}

type lensSearchSelection interface {
	kind() searchKind
	repository() domain.RepoRef
	isLensSearchSelection()
}

type repositoryLensSelection struct{ repo domain.RepoRef }

func (repositoryLensSelection) kind() searchKind             { return searchRepositories }
func (s repositoryLensSelection) repository() domain.RepoRef { return s.repo }
func (repositoryLensSelection) isLensSearchSelection()       {}

type threadLensSelection struct {
	repo     domain.RepoRef
	criteria threadSearchCriteria
}

func (s threadLensSelection) kind() searchKind           { return s.criteria.kind }
func (s threadLensSelection) repository() domain.RepoRef { return s.repo }
func (threadLensSelection) isLensSearchSelection()       {}

type codeLensSelection struct{ repo domain.RepoRef }

func (codeLensSelection) kind() searchKind             { return searchCodeDocuments }
func (s codeLensSelection) repository() domain.RepoRef { return s.repo }
func (codeLensSelection) isLensSearchSelection()       {}

type allLensSelection struct {
	repo     domain.RepoRef
	criteria threadSearchCriteria
}

func (allLensSelection) kind() searchKind             { return searchAll }
func (s allLensSelection) repository() domain.RepoRef { return s.repo }
func (allLensSelection) isLensSearchSelection()       {}

type lensSearchRequest struct {
	searchRead
	lens      string
	selection lensSearchSelection
}

func (r lensSearchRequest) read() searchRead           { return r.searchRead }
func (r lensSearchRequest) kind() searchKind           { return r.selection.kind() }
func (r lensSearchRequest) repository() domain.RepoRef { return r.selection.repository() }
func (lensSearchRequest) isParsedSearchRequest()       {}

func parseServiceSearchRequest(query string, opts contracts.SearchOptions) (parsedSearchRequest, error) {
	var repo domain.RepoRef
	if strings.TrimSpace(opts.Repo) != "" {
		parsed, err := domain.ParseRepoRef(opts.Repo)
		if err != nil {
			return nil, fmt.Errorf("invalid repository filter %q: %w", opts.Repo, err)
		}
		repo = parsed
	}
	return parseSearchRequest(query, opts, repo)
}

// parseSearchRequest consumes a loose boundary representation and returns one
// concrete operation. Callers must not pass SearchOptions farther inward.
func parseSearchRequest(query string, opts contracts.SearchOptions, repo domain.RepoRef) (parsedSearchRequest, error) {
	page, err := corpus.ParseSearchPage(opts.Limit, opts.Cursor)
	if err != nil {
		return nil, err
	}
	kind, err := parseSearchKind(opts.Kind)
	if err != nil {
		return nil, err
	}
	order, err := corpus.ParseSearchOrder(opts.Sort)
	if err != nil {
		return nil, err
	}
	match, err := corpus.ParseTermMatch(opts.MatchMode)
	if err != nil {
		return nil, err
	}
	state, err := corpus.ParseThreadStateFilter(opts.State)
	if err != nil {
		return nil, err
	}
	stateReason, err := corpus.ParseThreadStateReason(opts.StateReason)
	if err != nil {
		return nil, err
	}
	if state.IsOpen() && !stateReason.IsAny() {
		return nil, errors.New("state_reason cannot be combined with open state")
	}
	if !opts.UpdatedAfter.IsZero() && !opts.UpdatedBefore.IsZero() && opts.UpdatedBefore.Before(opts.UpdatedAfter) {
		return nil, errors.New("updated_before must not be earlier than updated_after")
	}
	criteria := threadSearchCriteria{
		kind: kind, state: state, stateReason: stateReason, merge: corpus.MergeFilterFromPointer(opts.Merged),
		author: strings.TrimSpace(opts.Author), association: strings.TrimSpace(opts.Association),
		assignee: strings.TrimSpace(opts.Assignee), labels: normalizeSearchLabels(opts.Labels),
		updatedAfter: opts.UpdatedAfter, updatedBefore: opts.UpdatedBefore, order: order, match: match,
	}
	if kind == searchIssues && !criteria.merge.IsAny() {
		return nil, errors.New("merged filter is only valid for pull-request or combined thread search")
	}

	read := searchRead{query: strings.TrimSpace(query), page: page, snapshotToken: opts.SnapshotToken}
	lensName := strings.TrimSpace(opts.Lens)
	if lensName != "" {
		if page.Cursor() != "" {
			return nil, errors.New("cursor pagination cannot be combined with --lens because lens ranking is not cursor-stable")
		}
		if order.IsUpdated() {
			return nil, errors.New("sort cannot be combined with a lens because the lens defines the final ranking")
		}
		selection, err := parseLensSearchSelection(kind, repo, criteria)
		if err != nil {
			return nil, err
		}
		return lensSearchRequest{searchRead: read, lens: lensName, selection: selection}, nil
	}

	switch kind {
	case searchRepositories:
		if criteria.hasMetadataFilters() || match.IsAny() {
			return nil, errors.New("thread filters and match_mode are not supported for repository search")
		}
		if repo.IsValid() && page.Cursor() != "" {
			return nil, errors.New("cursor pagination is not supported for exact repository search")
		}
		return repositorySearchRequest{searchRead: read, repo: repo, order: order}, nil
	case searchCodeDocuments:
		if criteria.hasMetadataFilters() || match.IsAny() {
			return nil, errors.New("thread filters and match_mode are not supported for code search")
		}
		if order.IsUpdated() {
			return nil, errors.New("code search supports relevance order only")
		}
		return codeSearchRequest{searchRead: read, repo: repo}, nil
	case searchAll:
		return nil, errors.New("combined search is not supported because FTS ranks from different indexes are not comparable; choose repos, threads, issues, prs, or code")
	case searchThreads, searchIssues, searchPullRequests:
		return threadSearchRequest{searchRead: read, repo: repo, criteria: criteria}, nil
	default:
		return nil, errors.New("invalid parsed search kind")
	}
}

func parseLensSearchSelection(kind searchKind, repo domain.RepoRef, criteria threadSearchCriteria) (lensSearchSelection, error) {
	switch kind {
	case searchRepositories:
		if criteria.hasMetadataFilters() || criteria.match.IsAny() {
			return nil, errors.New("thread filters and match_mode are not supported for repository search")
		}
		return repositoryLensSelection{repo: repo}, nil
	case searchCodeDocuments:
		if criteria.hasMetadataFilters() || criteria.match.IsAny() {
			return nil, errors.New("thread filters and match_mode are not supported for code search")
		}
		return codeLensSelection{repo: repo}, nil
	case searchThreads, searchIssues, searchPullRequests:
		return threadLensSelection{repo: repo, criteria: criteria}, nil
	case searchAll:
		criteria.kind = searchThreads
		return allLensSelection{repo: repo, criteria: criteria}, nil
	default:
		return nil, errors.New("invalid parsed lens selection")
	}
}

func normalizeSearchLabels(labels []string) []string {
	normalized := make([]string, 0, len(labels))
	for _, label := range labels {
		if label = strings.TrimSpace(label); label != "" {
			normalized = append(normalized, label)
		}
	}
	return normalized
}
