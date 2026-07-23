package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// SearchGitHubRepositories performs one bounded live repository search and
// persists the returned metadata observations without fetching thread data.
func (r *MCPReader) SearchGitHubRepositories(ctx context.Context, in mcpserver.SearchGitHubRepositoriesInput) (mcpserver.SearchGitHubRepositoriesOutput, error) {
	query, interpretation, warnings, err := compileRepositorySearch(in)
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	if err := normalizeRepositorySearchPage(&in); err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	searcher, ok := reader.(github.RepositorySearcher)
	if !ok {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("configured GitHub reader does not support repository search")
	}
	result, err := searcher.SearchRepositories(ctx, github.RepositorySearchOptions{Query: query, Sort: in.Sort, Order: in.Order, PageOptions: github.PageOptions{Page: in.Page, PerPage: in.Limit}})
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	return r.persistRepositorySearch(ctx, in, query, interpretation, warnings, result)
}

func normalizeRepositorySearchPage(in *mcpserver.SearchGitHubRepositoriesInput) error {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if in.Sort != "" && in.Sort != "stars" && in.Sort != "forks" && in.Sort != "help-wanted-issues" && in.Sort != "updated" {
		return mcpserver.InvalidArgument("sort", "must be stars, forks, help-wanted-issues, or updated", map[string]any{"sort": "stars"})
	}
	if in.Order != "" && in.Order != "asc" && in.Order != "desc" {
		return mcpserver.InvalidArgument("order", "must be asc or desc", map[string]any{"order": "desc"})
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Page < 1 || in.Page > 1000 || (in.Page-1)*in.Limit >= 1000 {
		return mcpserver.InvalidArgument("page", "must keep the requested result offset below GitHub's 1,000-result cap", map[string]any{"page": 1, "limit": in.Limit})
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return mcpserver.InvalidArgument("response_format", "must be concise or detailed; use concise for discovery and detailed for finalist inspection", map[string]any{"response_format": "concise"})
	}
	return nil
}

func (r *MCPReader) persistRepositorySearch(ctx context.Context, in mcpserver.SearchGitHubRepositoriesInput, query, interpretation string, warnings []mcpserver.SearchWarning, result github.RepositorySearchResult) (mcpserver.SearchGitHubRepositoriesOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	out := repositorySearchOutput(in, query, interpretation, warnings, result)
	observedAt := r.now()
	for i, remote := range result.Items {
		payload, err := json.Marshal(remote)
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
		if err == nil {
			err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
		}
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		metadata := mcpserver.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(observedAt), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
		value := liveRepositorySearchMatch(remote, metadata, in.ResponseFormat)
		out.Items[i] = mcpserver.BatchItem[mcpserver.RepositorySearchMatch]{Key: remote.Owner + "/" + remote.Name, Status: "complete", Value: &value}
	}
	addRepositorySearchAction(&out, result.Items)
	return out, nil
}

func repositorySearchOutput(in mcpserver.SearchGitHubRepositoriesInput, query, interpretation string, warnings []mcpserver.SearchWarning, result github.RepositorySearchResult) mcpserver.SearchGitHubRepositoriesOutput {
	out := mcpserver.SearchGitHubRepositoriesOutput{Status: "complete", Query: query, Interpretation: interpretation, ResponseFormat: in.ResponseFormat, Page: in.Page, Total: result.Total, Incomplete: result.Incomplete, Warnings: warnings, Items: make([]mcpserver.BatchItem[mcpserver.RepositorySearchMatch], len(result.Items))}
	if result.Page.HasNext {
		out.NextPage = result.Page.NextPage
	} else if in.Page*in.Limit < result.Total && in.Page*in.Limit < 1000 {
		out.NextPage = in.Page + 1
	}
	if result.Incomplete {
		out.Status = "partial"
		out.Warnings = append(out.Warnings, mcpserver.SearchWarning{Code: "github_results_incomplete", Message: "GitHub reported that this search page may be incomplete.", Suggestion: "Narrow the filters before treating absence as evidence."})
	}
	if result.Total > 1000 {
		out.Warnings = append(out.Warnings, mcpserver.SearchWarning{Code: "github_search_cap", Message: "GitHub search exposes at most 1,000 results for a query.", Suggestion: "Narrow the date, topic, language, or star filters."})
	}
	return out
}

func addRepositorySearchAction(out *mcpserver.SearchGitHubRepositoriesOutput, items []github.Repository) {
	if len(items) == 0 {
		return
	}
	if len(items) > 50 {
		items = items[:50]
		out.Warnings = append(out.Warnings, mcpserver.SearchWarning{Code: "suggested_action_bounded", Message: "The suggested thread sync is limited to the first 50 results.", Suggestion: "Select repositories before synchronizing thread headers."})
	}
	repositories := make([]mcpserver.RepositoryRef, 0, len(items))
	for _, remote := range items {
		repositories = append(repositories, mcpserver.RepositoryRef{Owner: remote.Owner, Repo: remote.Name})
	}
	out.SuggestedActions = []mcpserver.SuggestedAction{{Tool: mcpserver.ToolSyncThreads, Reason: "Fetch open issue headers only for repositories selected from these metadata results.", Arguments: map[string]any{"selection": "repositories", "repositories": repositories, "state": "open"}}}
}

func liveRepositorySearchMatch(remote github.Repository, metadata mcpserver.RepositoryMetadataOutput, format string) mcpserver.RepositorySearchMatch {
	match := mcpserver.RepositorySearchMatch{Ref: "repository:" + remote.Owner + "/" + remote.Name, Owner: remote.Owner, Repo: remote.Name, Description: ptr(remote.Description), Language: ptr(remote.Language), Stars: ptr(remote.Stars), Metadata: metadata}
	if remote.PushedAt != nil {
		match.PushedAt = formatTime(*remote.PushedAt)
	}
	if format == "detailed" {
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

func compileRepositorySearch(in mcpserver.SearchGitHubRepositoriesInput) (string, string, []mcpserver.SearchWarning, error) {
	query, mode, warnings, structured, err := repositorySearchMode(in)
	if err != nil {
		return "", "", nil, err
	}
	if !structured {
		if strings.Contains(strings.ToLower(query), "in:readme") {
			warnings = append(warnings, readmeSearchWarning())
		}
		return query, "Search using " + mode + ".", warnings, nil
	}
	query, structuredWarnings, err := compileStructuredRepositorySearch(in)
	return query, "Search using structured repository filters.", append(warnings, structuredWarnings...), err
}

func repositorySearchMode(in mcpserver.SearchGitHubRepositoriesInput) (string, string, []mcpserver.SearchWarning, bool, error) {
	raw := strings.TrimSpace(in.RawQuery)
	structured := hasStructuredRepositorySearch(in)
	if raw != "" && structured {
		return "", "", nil, false, mcpserver.InvalidArgument("raw_query", "cannot be combined with structured filters; choose one input mode", map[string]any{"raw_query": "is:public language:go stars:>=100"})
	}
	if raw == "" && !structured {
		return "", "", nil, false, mcpserver.InvalidArgument("text", "provide raw_query or at least one structured filter such as text, topics, language, or pushed_after", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	if raw != "" {
		return raw, "advanced raw query", nil, false, nil
	}
	return "", "", nil, true, nil
}

func hasStructuredRepositorySearch(in mcpserver.SearchGitHubRepositoriesInput) bool {
	return strings.TrimSpace(in.Text) != "" || len(in.MatchFields) > 0 || len(in.Topics) > 0 || strings.TrimSpace(in.Language) != "" || in.StarsMin != 0 || in.StarsMax != 0 || in.CreatedAfter != "" || in.CreatedBefore != "" || in.PushedAfter != "" || in.PushedBefore != "" || in.Archived != nil || in.Fork != nil
}

func compileStructuredRepositorySearch(in mcpserver.SearchGitHubRepositoriesInput) (string, []mcpserver.SearchWarning, error) {
	if err := validateRepositoryMatchFields(in.Text, in.MatchFields); err != nil {
		return "", nil, err
	}
	parts := appendRepositoryText(nil, in.Text, in.MatchFields)
	warnings := []mcpserver.SearchWarning{}
	if repositorySearchContains(in.MatchFields, "readme") {
		warnings = append(warnings, readmeSearchWarning())
	}
	for _, topic := range in.Topics {
		if strings.TrimSpace(topic) == "" {
			return "", nil, mcpserver.InvalidArgument("topics", "must not contain blank values", map[string]any{"topics": []string{"go", "github-actions"}})
		}
		parts = append(parts, "topic:"+quoteSearchTerm(topic))
	}
	if language := strings.TrimSpace(in.Language); language != "" {
		parts = append(parts, "language:"+quoteSearchTerm(language))
	}
	if in.StarsMin < 0 || in.StarsMax < 0 || in.StarsMax > 0 && in.StarsMin > in.StarsMax {
		return "", nil, mcpserver.InvalidArgument("stars_min", "stars bounds must be non-negative and stars_min cannot exceed stars_max", map[string]any{"stars_min": 200, "stars_max": 10000})
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
		return mcpserver.InvalidArgument("match_fields", "requires text", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	for _, field := range fields {
		if field != "name" && field != "description" && field != "readme" {
			return mcpserver.InvalidArgument("match_fields", "values must be name, description, or readme", map[string]any{"match_fields": []string{"name", "description"}})
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

func readmeSearchWarning() mcpserver.SearchWarning {
	return mcpserver.SearchWarning{Code: "broad_readme_match", Message: "README matching can include incidental mentions and unrelated repositories.", Suggestion: "Prefer name, description, topic, or language filters for discovery."}
}
func quoteSearchTerm(value string) string {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, " \t") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}

func appendNumericRange(parts []string, name string, minimum, maximum int) []string {
	switch {
	case minimum > 0 && maximum > 0:
		return append(parts, fmt.Sprintf("%s:%d..%d", name, minimum, maximum))
	case minimum > 0:
		return append(parts, fmt.Sprintf("%s:>=%d", name, minimum))
	case maximum > 0:
		return append(parts, fmt.Sprintf("%s:<=%d", name, maximum))
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
