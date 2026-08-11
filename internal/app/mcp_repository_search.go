package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type repositorySearchSort uint8

const (
	repositorySearchBestMatch repositorySearchSort = iota
	repositorySearchStars
	repositorySearchForks
	repositorySearchHelpWanted
	repositorySearchUpdated
)

func parseRepositorySearchSort(value string) (repositorySearchSort, error) {
	switch strings.TrimSpace(value) {
	case "":
		return repositorySearchBestMatch, nil
	case "stars":
		return repositorySearchStars, nil
	case "forks":
		return repositorySearchForks, nil
	case "help-wanted-issues":
		return repositorySearchHelpWanted, nil
	case "updated":
		return repositorySearchUpdated, nil
	default:
		return 0, errors.New("unsupported repository search sort")
	}
}

func (s repositorySearchSort) String() string {
	return [...]string{"", "stars", "forks", "help-wanted-issues", "updated"}[s]
}

type githubRepositorySearchRequest struct {
	query          string
	interpretation string
	warnings       []mcpcontract.SearchWarning
	sort           repositorySearchSort
	order          githubSearchOrder
	page           githubSearchPage
	format         responseFormat
}

func parseRepositorySearchInput(in mcpcontract.SearchGitHubRepositoriesInput) (githubRepositorySearchRequest, error) {
	query, interpretation, warnings, err := compileRepositorySearch(in)
	if err != nil {
		return githubRepositorySearchRequest{}, err
	}
	page, pageProblem := parseGitHubSearchPage(in.Limit, in.Page)
	if pageProblem == githubSearchLimitInvalid {
		return githubRepositorySearchRequest{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	sortMode, err := parseRepositorySearchSort(in.Sort)
	if err != nil {
		return githubRepositorySearchRequest{}, mcpcontract.InvalidArgument("sort", "must be stars, forks, help-wanted-issues, or updated", map[string]any{"sort": "stars"})
	}
	order, err := parseGitHubSearchOrder(in.Order, githubSearchOrderUnspecified)
	if err != nil {
		return githubRepositorySearchRequest{}, mcpcontract.InvalidArgument("order", "must be asc or desc", map[string]any{"order": "desc"})
	}
	if pageProblem == githubSearchPageInvalid {
		limit := in.Limit
		if limit == 0 {
			limit = 20
		}
		return githubRepositorySearchRequest{}, mcpcontract.InvalidArgument("page", "must keep the requested result offset below GitHub's 1,000-result cap", map[string]any{"page": 1, "limit": limit})
	}
	format, err := parseResponseFormat(in.ResponseFormat)
	if err != nil {
		return githubRepositorySearchRequest{}, mcpcontract.InvalidArgument("response_format", "must be concise or detailed; use concise for discovery and detailed for finalist inspection", map[string]any{"response_format": "concise"})
	}
	return githubRepositorySearchRequest{
		query: query, interpretation: interpretation, warnings: append([]mcpcontract.SearchWarning(nil), warnings...),
		sort: sortMode, order: order, page: page, format: format,
	}, nil
}

// SearchGitHubRepositories performs one bounded live repository search and
// persists the returned metadata observations without fetching thread data.
func (r *MCPReader) SearchGitHubRepositories(ctx context.Context, in mcpcontract.SearchGitHubRepositoriesInput) (mcpcontract.SearchGitHubRepositoriesOutput, error) {
	request, err := parseRepositorySearchInput(in)
	if err != nil {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	searcher, ok := reader.(github.RepositorySearcher)
	if !ok {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, errors.New("configured GitHub reader does not support repository search")
	}
	result, err := searcher.SearchRepositories(ctx, github.RepositorySearchOptions{
		Query: request.query, Sort: request.sort.String(), Order: request.order.String(),
		PageOptions: github.PageOptions{Page: request.page.number, PerPage: request.page.limit},
	})
	if err != nil {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	return r.persistRepositorySearch(ctx, request, result)
}

func (r *MCPReader) persistRepositorySearch(ctx context.Context, request githubRepositorySearchRequest, result github.RepositorySearchResult) (mcpcontract.SearchGitHubRepositoriesOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	out := repositorySearchOutput(request, result)
	observedAt := r.now()
	repositoryIDs := make([]int64, 0, len(result.Items))
	for i, remote := range result.Items {
		payload, err := json.Marshal(remote)
		if err != nil {
			return mcpcontract.SearchGitHubRepositoriesOutput{}, err
		}
		stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
		if err == nil {
			err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
		}
		if err != nil {
			return mcpcontract.SearchGitHubRepositoriesOutput{}, err
		}
		repositoryIDs = append(repositoryIDs, stored.ID)
		metadata := mcpcontract.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(observedAt), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
		value := liveRepositorySearchMatch(remote, metadata, request.format)
		value.DossierStatus = "missing"
		out.Items[i] = mcpcontract.BatchItem[mcpcontract.RepositorySearchMatch]{Key: remote.Owner + "/" + remote.Name, Status: "complete", Value: &value}
	}
	dossiers, err := c.GetLatestDossierMetadataBatch(ctx, repositoryIDs)
	if err != nil {
		return mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	for i, repositoryID := range repositoryIDs {
		if dossier, ok := dossiers[repositoryID]; ok && out.Items[i].Value != nil {
			out.Items[i].Value.DossierStatus = "available"
			out.Items[i].Value.DossierAsOf = formatTime(dossier.AsOf)
		}
	}
	addRepositorySearchAction(&out, result.Items)
	return out, nil
}

func repositorySearchOutput(request githubRepositorySearchRequest, result github.RepositorySearchResult) mcpcontract.SearchGitHubRepositoriesOutput {
	out := mcpcontract.SearchGitHubRepositoriesOutput{Status: "complete", Query: request.query, Interpretation: request.interpretation, ResponseFormat: request.format.String(), Page: request.page.number, Total: result.Total, Incomplete: result.Incomplete, Warnings: append([]mcpcontract.SearchWarning(nil), request.warnings...), Items: make([]mcpcontract.BatchItem[mcpcontract.RepositorySearchMatch], len(result.Items))}
	if result.Page.HasNext {
		out.NextPage = result.Page.NextPage
	} else if request.page.number*request.page.limit < result.Total && request.page.number*request.page.limit < 1000 {
		out.NextPage = request.page.number + 1
	}
	if result.Incomplete {
		out.Status = "partial"
		out.Warnings = append(out.Warnings, mcpcontract.SearchWarning{Code: "github_results_incomplete", Message: "GitHub reported that this search page may be incomplete.", Suggestion: "Narrow the filters before treating absence as evidence."})
	}
	if result.Total > 1000 {
		out.Warnings = append(out.Warnings, mcpcontract.SearchWarning{Code: "github_search_cap", Message: "GitHub search exposes at most 1,000 results for a query.", Suggestion: "Narrow the date, topic, language, or star filters."})
	}
	return out
}

func addRepositorySearchAction(out *mcpcontract.SearchGitHubRepositoriesOutput, items []github.Repository) {
	if len(items) == 0 {
		return
	}
	if len(items) > 50 {
		items = items[:50]
		out.Warnings = append(out.Warnings, mcpcontract.SearchWarning{Code: "suggested_action_bounded", Message: "The suggested thread sync is limited to the first 50 results.", Suggestion: "Select repositories before synchronizing thread headers."})
	}
	repositories := make([]mcpcontract.RepositoryRef, 0, len(items))
	for _, remote := range items {
		repositories = append(repositories, mcpcontract.RepositoryRef{Owner: remote.Owner, Repo: remote.Name})
	}
	out.RecoveryPlans = []mcpcontract.RecoveryPlan{{
		Version: mcpcontract.RecoveryPlanVersion, Reason: "coverage_stale", Message: "Fetch open issue headers only for repositories selected from these metadata results.",
		Then: []mcpcontract.ToolCall{mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: repositories, State: "open"})},
	}}
}

func liveRepositorySearchMatch(remote github.Repository, metadata mcpcontract.RepositoryMetadataOutput, format responseFormat) mcpcontract.RepositorySearchMatch {
	match := mcpcontract.RepositorySearchMatch{Ref: "repository:" + remote.Owner + "/" + remote.Name, Owner: remote.Owner, Repo: remote.Name, Description: ptr(remote.Description), Language: ptr(remote.Language), Stars: ptr(remote.Stars), Metadata: metadata}
	if remote.PushedAt != nil {
		match.PushedAt = formatTime(*remote.PushedAt)
	}
	if format.includesDetails() {
		match.DefaultBranch = ptr(remote.DefaultBranch)
		match.License = ptr(remote.License)
		match.Topics = append([]string(nil), remote.Topics...)
		match.Watchers = ptr(remote.Watchers)
		match.Forks = ptr(remote.Forks)
		match.OpenIssues = ptr(remote.OpenIssues)
		match.Archived = ptr(remote.Archived)
		match.Fork = ptr(remote.Fork)
	}
	return match
}

func compileRepositorySearch(in mcpcontract.SearchGitHubRepositoriesInput) (string, string, []mcpcontract.SearchWarning, error) {
	raw := strings.TrimSpace(in.RawQuery)
	structured := hasStructuredRepositorySearch(in)
	if raw != "" && structured {
		return "", "", nil, mcpcontract.InvalidArgument("raw_query", "cannot be combined with structured filters; choose one input mode", map[string]any{"raw_query": "is:public language:go stars:>=100"})
	}
	if raw == "" && !structured {
		return "", "", nil, mcpcontract.InvalidArgument("text", "provide raw_query or at least one structured filter such as text, topics, language, or pushed_after", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	if raw != "" {
		warnings := []mcpcontract.SearchWarning{}
		if strings.Contains(strings.ToLower(raw), "in:readme") {
			warnings = append(warnings, readmeSearchWarning())
		}
		return raw, "Search using advanced raw query.", warnings, nil
	}
	query, warnings, err := compileStructuredRepositorySearch(in)
	return query, "Search using structured repository filters.", warnings, err
}

func hasStructuredRepositorySearch(in mcpcontract.SearchGitHubRepositoriesInput) bool {
	return strings.TrimSpace(in.Text) != "" || len(in.MatchFields) > 0 || len(in.Topics) > 0 || strings.TrimSpace(in.Language) != "" || in.StarsMin != nil || in.StarsMax != nil || in.CreatedAfter != "" || in.CreatedBefore != "" || in.PushedAfter != "" || in.PushedBefore != "" || in.Archived != nil || in.Fork != nil
}

func compileStructuredRepositorySearch(in mcpcontract.SearchGitHubRepositoriesInput) (string, []mcpcontract.SearchWarning, error) {
	if err := validateRepositoryMatchFields(in.Text, in.MatchFields); err != nil {
		return "", nil, err
	}
	parts := appendRepositoryText(nil, in.Text, in.MatchFields)
	warnings := []mcpcontract.SearchWarning{}
	if repositorySearchContains(in.MatchFields, "readme") {
		warnings = append(warnings, readmeSearchWarning())
	}
	for _, topic := range in.Topics {
		if strings.TrimSpace(topic) == "" {
			return "", nil, mcpcontract.InvalidArgument("topics", "must not contain blank values", map[string]any{"topics": []string{"go", "github-actions"}})
		}
		parts = append(parts, "topic:"+quoteSearchTerm(topic))
	}
	if language := strings.TrimSpace(in.Language); language != "" {
		parts = append(parts, "language:"+quoteSearchTerm(language))
	}
	if in.StarsMin != nil && *in.StarsMin < 0 || in.StarsMax != nil && *in.StarsMax < 0 ||
		in.StarsMin != nil && in.StarsMax != nil && *in.StarsMin > *in.StarsMax {
		return "", nil, mcpcontract.InvalidArgument("stars_min", "stars bounds must be non-negative and stars_min cannot exceed stars_max", map[string]any{"stars_min": 200, "stars_max": 10000})
	}
	parts = appendNumericRange(parts, "stars", in.StarsMin, in.StarsMax)
	var err error
	parts, err = appendDateRange(parts, "created", in.CreatedAfter, in.CreatedBefore)
	if err == nil {
		parts, err = appendDateRange(parts, "pushed", in.PushedAfter, in.PushedBefore)
	}
	if err != nil {
		return "", nil, err
	}
	if in.Archived != nil {
		parts = append(parts, fmt.Sprintf("archived:%t", *in.Archived))
	}
	if in.Fork != nil {
		parts = append(parts, fmt.Sprintf("fork:%t", *in.Fork))
	}
	return strings.Join(parts, " "), warnings, nil
}

func validateRepositoryMatchFields(text string, fields []string) error {
	if len(fields) > 0 && strings.TrimSpace(text) == "" {
		return mcpcontract.InvalidArgument("match_fields", "requires text", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	for _, field := range fields {
		if field != "name" && field != "description" && field != "readme" {
			return mcpcontract.InvalidArgument("match_fields", "values must be name, description, or readme", map[string]any{"match_fields": []string{"name", "description"}})
		}
	}
	return nil
}

func appendRepositoryText(parts []string, text string, fields []string) []string {
	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, quoteSearchTerm(text))
	}
	if len(fields) > 0 {
		parts = append(parts, "in:"+strings.Join(fields, ","))
	}
	return parts
}

func repositorySearchContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readmeSearchWarning() mcpcontract.SearchWarning {
	return mcpcontract.SearchWarning{Code: "broad_readme_match", Message: "README matching can include incidental mentions and unrelated repositories.", Suggestion: "Prefer name, description, topic, or language filters for discovery."}
}
func quoteSearchTerm(value string) string {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, " \t") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}

func appendNumericRange(parts []string, name string, minimum, maximum *int) []string {
	switch {
	case minimum != nil && maximum != nil:
		return append(parts, fmt.Sprintf("%s:%d..%d", name, *minimum, *maximum))
	case minimum != nil:
		return append(parts, fmt.Sprintf("%s:>=%d", name, *minimum))
	case maximum != nil:
		return append(parts, fmt.Sprintf("%s:<=%d", name, *maximum))
	default:
		return parts
	}
}

func appendDateRange(parts []string, name, after, before string) ([]string, error) {
	after = strings.TrimSpace(after)
	before = strings.TrimSpace(before)
	for _, value := range []string{after, before} {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return nil, fmt.Errorf("%s dates must use YYYY-MM-DD: %w", name, err)
			}
		}
	}
	if after != "" && before != "" && after > before {
		return nil, fmt.Errorf("%s_after cannot be later than %s_before", name, name)
	}
	switch {
	case after != "" && before != "":
		return append(parts, name+":"+after+".."+before), nil
	case after != "":
		return append(parts, name+":>="+after), nil
	case before != "":
		return append(parts, name+":<="+before), nil
	default:
		return parts, nil
	}
}
