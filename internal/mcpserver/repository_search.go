package mcpserver

// SearchGitHubRepositoriesInput defines one bounded live GitHub search.
type SearchGitHubRepositoriesInput struct {
	RawQuery       string   `json:"raw_query,omitempty" jsonschema:"Advanced raw GitHub query; exclusive with filters"`
	Text           string   `json:"text,omitempty" jsonschema:"Text to match"`
	MatchFields    []string `json:"match_fields,omitempty" jsonschema:"Text fields: name, description, or readme"`
	Topics         []string `json:"topics,omitempty" jsonschema:"Topics that must all match"`
	Language       string   `json:"language,omitempty" jsonschema:"Primary language"`
	StarsMin       int      `json:"stars_min,omitempty" jsonschema:"Minimum stargazer count"`
	StarsMax       int      `json:"stars_max,omitempty" jsonschema:"Maximum stargazer count"`
	CreatedAfter   string   `json:"created_after,omitempty" jsonschema:"Created on or after YYYY-MM-DD"`
	CreatedBefore  string   `json:"created_before,omitempty" jsonschema:"Created on or before YYYY-MM-DD"`
	PushedAfter    string   `json:"pushed_after,omitempty" jsonschema:"Pushed on or after YYYY-MM-DD"`
	PushedBefore   string   `json:"pushed_before,omitempty" jsonschema:"Pushed on or before YYYY-MM-DD"`
	Archived       *bool    `json:"archived,omitempty" jsonschema:"Archived state"`
	Fork           *bool    `json:"fork,omitempty" jsonschema:"Fork state"`
	Sort           string   `json:"sort,omitempty" jsonschema:"Optional GitHub sort: stars, forks, help-wanted-issues, or updated"`
	Order          string   `json:"order,omitempty" jsonschema:"Sort order: asc or desc"`
	Limit          int      `json:"limit,omitempty" jsonschema:"Results per page from 1 to 100"`
	Page           int      `json:"page,omitempty" jsonschema:"Result page within GitHub's 1,000-result cap"`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"concise or detailed"`
}

// SuggestedAction describes a non-mandatory follow-up with reusable arguments.
type SuggestedAction struct {
	Tool      string         `json:"tool"`
	Reason    string         `json:"reason"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// SearchWarning explains a request-specific limitation and how to improve it.
type SearchWarning struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// RepositorySearchMatch is a token-bounded live repository search result.
type RepositorySearchMatch struct {
	Ref           string                   `json:"ref"`
	Owner         string                   `json:"owner"`
	Repo          string                   `json:"repo"`
	Description   *string                  `json:"description,omitempty"`
	Language      *string                  `json:"language,omitempty"`
	Stars         *int                     `json:"stars,omitempty"`
	PushedAt      string                   `json:"pushed_at,omitempty"`
	Metadata      RepositoryMetadataOutput `json:"metadata"`
	DefaultBranch *string                  `json:"default_branch,omitempty"`
	License       *string                  `json:"license,omitempty"`
	Topics        []string                 `json:"topics,omitempty"`
	Watchers      *int                     `json:"watchers,omitempty"`
	Forks         *int                     `json:"forks,omitempty"`
	OpenIssues    *int                     `json:"open_issues,omitempty"`
	Archived      *bool                    `json:"archived,omitempty"`
	Fork          *bool                    `json:"fork,omitempty"`
}

// SearchGitHubRepositoriesOutput contains search results and completeness metadata.
type SearchGitHubRepositoriesOutput struct {
	Status           string                             `json:"status"`
	Query            string                             `json:"query"`
	Interpretation   string                             `json:"interpretation"`
	ResponseFormat   string                             `json:"response_format"`
	Page             int                                `json:"page"`
	NextPage         int                                `json:"next_page,omitempty"`
	Total            int                                `json:"total"`
	Incomplete       bool                               `json:"incomplete"`
	Items            []BatchItem[RepositorySearchMatch] `json:"items"`
	Warnings         []SearchWarning                    `json:"warnings,omitempty"`
	SuggestedActions []SuggestedAction                  `json:"suggested_actions,omitempty"`
}
