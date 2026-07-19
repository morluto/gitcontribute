package cli

// RecordContributionOptions describes a prepared contribution to persist.
type RecordContributionOptions struct {
	OpportunityID string
	Kind          string
	Title         string
	Body          string
	Reference     string
	ReferenceURL  string
}

// ContributionResult is the stored representation of a prepared contribution.
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

// ListContributionsOptions filters and bounds contribution history.
type ListContributionsOptions struct {
	OpportunityID string
	Kind          string
	Limit         int
}

// ContributionListResult contains a bounded contribution history page.
type ContributionListResult struct {
	Contributions []ContributionResult `json:"contributions"`
	Limit         int                  `json:"limit"`
	Total         int                  `json:"total"`
}

// RecordContributionOutcomeOptions describes an outcome to attach to a contribution.
type RecordContributionOutcomeOptions struct {
	ContributionID string
	Outcome        string
	Reason         string
}

// ContributionOutcomeResult is a stored contribution outcome.
type ContributionOutcomeResult struct {
	ID             string `json:"id"`
	ContributionID string `json:"contribution_id"`
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
	SourceEventAt  string `json:"source_event_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// ContributionOutcomeListResult contains all stored outcomes for one contribution.
type ContributionOutcomeListResult struct {
	ContributionID string                      `json:"contribution_id"`
	Outcomes       []ContributionOutcomeResult `json:"outcomes"`
}
