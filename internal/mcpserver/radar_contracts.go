package mcpserver

// RankOpportunitiesInput bounds ranking across stored repositories.
type RankOpportunitiesInput struct {
	Repositories            []RepositoryRef `json:"repositories" jsonschema:"Required 1-50 stored repositories"`
	Limit                   int             `json:"limit,omitempty" jsonschema:"Result bound from 1-100"`
	MaxResultsPerRepository int             `json:"max_results_per_repository,omitempty" jsonschema:"Per-repository bound from 1-100"`
}

// OpportunityCandidateOutput describes one ranked contribution candidate.
type OpportunityCandidateOutput struct {
	Rank               int                            `json:"rank"`
	Ref                string                         `json:"ref"`
	Repo               string                         `json:"repo"`
	Number             int                            `json:"number"`
	Title              string                         `json:"title"`
	URL                string                         `json:"url"`
	Score              int                            `json:"score"`
	Eligibility        string                         `json:"eligibility"`
	Confidence         string                         `json:"confidence"`
	PositiveSignals    []string                       `json:"positive_signals,omitempty"`
	Risks              []string                       `json:"risks,omitempty"`
	Blockers           []string                       `json:"blockers,omitempty"`
	Unknowns           []string                       `json:"unknowns,omitempty"`
	LinkedPullRequests []int                          `json:"linked_pull_requests,omitempty"`
	RelatedWork        []OpportunityRelatedWorkOutput `json:"related_work,omitempty"`
	SourceUpdatedAt    string                         `json:"source_updated_at,omitempty"`
}

// RepositoryOpportunitySummaryOutput reports ranking coverage for one repository.
type RepositoryOpportunitySummaryOutput struct {
	Repo             string `json:"repo"`
	TotalOpenIssues  int    `json:"total_open_issues"`
	Considered       int    `json:"considered"`
	Returned         int    `json:"returned"`
	Truncated        bool   `json:"truncated"`
	PopulationCapped bool   `json:"population_capped"`
}

// RankOpportunitiesOutput combines deterministic cross-repository ranking with
// per-repository coverage or availability results.
type RankOpportunitiesOutput struct {
	Status       string                                          `json:"status"`
	Candidates   []OpportunityCandidateOutput                    `json:"candidates"`
	Repositories []BatchItem[RepositoryOpportunitySummaryOutput] `json:"repositories"`
	GeneratedAt  string                                          `json:"generated_at"`
	Total        int                                             `json:"total"`
	Truncated    bool                                            `json:"truncated"`
}
