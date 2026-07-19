package cli

type RecordContributionOptions struct {
	OpportunityID string
	Kind          string
	Title         string
	Body          string
	Reference     string
	ReferenceURL  string
}

type ContributionResult struct {
	ID            string         `json:"id"`
	OpportunityID string         `json:"opportunity_id"`
	Kind          string         `json:"kind"`
	Title         string         `json:"title"`
	Body          string         `json:"body,omitempty"`
	Reference     string         `json:"reference,omitempty"`
	ReferenceURL  string         `json:"reference_url,omitempty"`
	PreparedAt    string         `json:"prepared_at"`
	SubmittedAt   string         `json:"submitted_at,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type ListContributionsOptions struct {
	OpportunityID string
	Kind          string
	Limit         int
}

type ContributionListResult struct {
	Contributions []ContributionResult `json:"contributions"`
	Limit         int                  `json:"limit"`
	Total         int                  `json:"total"`
}

type RecordContributionOutcomeOptions struct {
	ContributionID string
	Outcome        string
	Reason         string
}

type ContributionOutcomeResult struct {
	ID             string `json:"id"`
	ContributionID string `json:"contribution_id"`
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
	SourceEventAt  string `json:"source_event_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type ContributionOutcomeListResult struct {
	ContributionID string                      `json:"contribution_id"`
	Outcomes       []ContributionOutcomeResult `json:"outcomes"`
}
