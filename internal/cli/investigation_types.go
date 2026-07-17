package cli

type investigationCmd struct {
	Start       startInvestigationCmd       `cmd:"" help:"Start an investigation"`
	StartThread startThreadInvestigationCmd `cmd:"" name:"start-thread" help:"Atomically start from one stored issue or pull request"`
	Show        showInvestigationCmd        `cmd:"" help:"Show an investigation"`
	List        listInvestigationCmd        `cmd:"" help:"List investigations"`
}

// InvestigationResult is a single investigation view.
type InvestigationResult struct {
	ID               string                `json:"id"`
	Repo             RepoRef               `json:"repo"`
	CommitSHA        string                `json:"commit_sha,omitempty"`
	Lens             string                `json:"lens,omitempty"`
	Status           string                `json:"status"`
	ThreadBaseline   *ThreadBaselineResult `json:"thread_baseline,omitempty"`
	SeedHypothesisID string                `json:"seed_hypothesis_id,omitempty"`
	AuditTrail       []WorkflowAuditResult `json:"audit_trail,omitempty"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
}

// HypothesisResult is a single hypothesis view.
type HypothesisResult struct {
	ID              string                    `json:"id"`
	InvestigationID string                    `json:"investigation_id"`
	Title           string                    `json:"title"`
	Description     string                    `json:"description"`
	Category        string                    `json:"category"`
	Status          string                    `json:"status"`
	SourceRefs      []WorkflowSourceRefResult `json:"source_refs,omitempty"`
	Links           []WorkflowLinkResult      `json:"links,omitempty"`
	AuditTrail      []WorkflowAuditResult     `json:"audit_trail,omitempty"`
	CreatedAt       string                    `json:"created_at"`
	UpdatedAt       string                    `json:"updated_at"`
}
