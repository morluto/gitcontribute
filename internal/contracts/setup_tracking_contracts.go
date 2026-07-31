package contracts

import (
	"context"
	"encoding/json"
)

// SetupService exposes local onboarding and client-registration operations.
// Setup may install a private MCP runtime, invoke npm for the global CLI, write
// local configuration, and initialize the corpus. It must not perform GitHub
// network access or execute repository-controlled code.
type SetupService interface {
	DiscoverSetup(ctx context.Context) (*SetupDiscovery, error)
	Setup(ctx context.Context, opts SetupOptions) (*SetupReport, error)
	SetupWithProgress(ctx context.Context, opts SetupOptions, observer SetupObserver) (*SetupReport, error)
}

// SetupObserver receives repository-owned progress events. Implementations must
// return promptly and must not alter setup behavior.
type SetupObserver interface {
	SetupStarted(phase SetupPhase)
	SetupCompleted(step SetupStep)
}

// SetupPhase identifies a long-running application operation.
type SetupPhase string

const (
	// SetupPhaseCLI installs and verifies the persistent terminal command.
	SetupPhaseCLI SetupPhase = "cli"
	// SetupPhaseMCPRuntime installs the private native runtime used by MCP-only setup.
	SetupPhaseMCPRuntime SetupPhase = "mcp-runtime"
	// SetupPhaseConfiguration writes shared local configuration.
	SetupPhaseConfiguration SetupPhase = "configuration"
	// SetupPhaseCorpus initializes the local corpus.
	SetupPhaseCorpus SetupPhase = "corpus"
	// SetupPhaseClients registers the MCP server with selected clients.
	SetupPhaseClients SetupPhase = "clients"
	// SetupPhaseRepository adds the optional initial repository source.
	SetupPhaseRepository SetupPhase = "repository"
	// SetupPhaseVerification checks the completed local installation.
	SetupPhaseVerification SetupPhase = "verification"
)

// SetupDiscovery is a read-only snapshot used to choose sensible onboarding
// defaults. Discovery never authenticates, performs network access, or writes
// configuration.
type SetupDiscovery struct {
	Version               string
	Clients               []SetupClientDiscovery
	ConfiguredTokenSource string
	ConfiguredTokenKey    string
	GitHubCLIAvailable    bool
	EnvironmentKeyPresent bool
}

// SetupClientDiscovery describes one supported coding client and the exact
// configuration file GitContribute would update.
type SetupClientDiscovery struct {
	Name       string
	Path       string
	Detected   bool
	Registered bool
	Error      string
}

// SetupMode selects one complete onboarding strategy.
type SetupMode string

const (
	// SetupModeMCP installs private MCP access without a global CLI command.
	SetupModeMCP SetupMode = "mcp"
	// SetupModeCLI installs the global CLI without coding-agent configuration.
	SetupModeCLI SetupMode = "cli"
	// SetupModeBoth installs the global CLI and configures coding-agent MCP access.
	SetupModeBoth SetupMode = "both"
)

// SetupOptions selects one access mode and its explicit targets. DryRun plans
// the selected mode without invoking npm or writing local state.
type SetupOptions struct {
	Remove     bool
	Mode       SetupMode
	Clients    []string
	AllClients bool

	TokenSource    string
	TokenSourceKey string
	Repository     string
	DryRun         bool
	// Version is the release used for persistent CLI or private MCP runtime
	// installation. Empty values inherit the running service version.
	Version string
	// Executable is the packaged native program copied for MCP-only setup. It is
	// injectable so installation behavior can be tested without copying the test
	// process itself.
	Executable string
}

// SetupStep describes one independently observable setup effect. Status is a
// stable human-readable state such as "would install", "installed",
// "configured", "not installed", or "failed".
type SetupStep struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// SetupAuthentication describes the credential source recorded by setup. It
// never contains a credential value and does not imply that credentials were
// read or validated.
type SetupAuthentication struct {
	Method string `json:"method"`
	Key    string `json:"key,omitempty"`
}

// SetupMCPCommand preserves the executable and argument boundaries registered
// with coding clients.
type SetupMCPCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// SetupReport records the effects attempted by setup. MCPCommand is populated
// only when MCP was selected. A report may contain both successful and failed
// independent steps.
type SetupReport struct {
	Operation         string                  `json:"operation"`
	DryRun            bool                    `json:"dry_run"`
	MCPCommand        *SetupMCPCommand        `json:"mcp_command,omitempty"`
	MCPCommandPending bool                    `json:"mcp_command_pending,omitempty"`
	RestartClients    []string                `json:"restart_clients,omitempty"`
	Authentication    *SetupAuthentication    `json:"authentication,omitempty"`
	Corpus            *CorpusInspectionResult `json:"corpus,omitempty"`
	Steps             []SetupStep             `json:"steps"`
}

// SyncResult reports the outcome of syncing a repository.
type SyncThreadRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Kind   string `json:"kind"`
	Number int    `json:"number"`
}

type SyncResult struct {
	Repo            RepoRef         `json:"repo"`
	Threads         []SyncThreadRef `json:"threads,omitempty"`
	Updated         int             `json:"updated"`
	Requests        int             `json:"requests"`
	PlannedRequests int             `json:"planned_requests"`
	RequestBudget   int             `json:"request_budget"`
	Capped          bool            `json:"request_capped"`
	Message         string          `json:"message"`
}

// SyncPlanResult is the conservative request ceiling computed before a sync
// obtains a GitHub reader or writes the corpus.
type SyncPlanResult struct {
	Repo                 RepoRef `json:"repo"`
	FixedRequests        int     `json:"fixed_requests"`
	ThreadRequestCeiling int     `json:"thread_request_ceiling"`
	PlannedRequests      int     `json:"planned_requests"`
	RequestBudget        int     `json:"request_budget"`
	MaxPages             int     `json:"max_pages"`
	ExactThreads         int     `json:"exact_threads"`
}

// MetadataExportOptions bounds a local tracking metadata export.
type MetadataExportOptions struct {
	Limit int
}

// MetadataExportResult contains the exported tracking bundle and record counts.
type MetadataExportResult struct {
	SchemaVersion        int             `json:"schema_version"`
	Data                 json.RawMessage `json:"data"`
	TriageEvents         int             `json:"triage_events"`
	Contributions        int             `json:"contributions"`
	ContributionOutcomes int             `json:"contribution_outcomes"`
	Evidence             int             `json:"evidence"`
}

// MetadataImportOptions carries a serialized local tracking bundle.
type MetadataImportOptions struct {
	Data []byte
}

// MetadataImportResult reports the imported bundle version and record counts.
type MetadataImportResult struct {
	SchemaVersion        int `json:"schema_version"`
	TriageEvents         int `json:"triage_events"`
	Contributions        int `json:"contributions"`
	ContributionOutcomes int `json:"contribution_outcomes"`
	Evidence             int `json:"evidence"`
}

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

// UpgradeStage reports one inspectable upgrade stage.
type UpgradeStage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message,omitempty"`
}

// UpgradeConfiguredClient reports one coding client's runtime registration.
type UpgradeConfiguredClient struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UpgradeReport describes installation, compatibility, activation, and rollback.
type UpgradeReport struct {
	Context           string                    `json:"context"`
	Current           string                    `json:"current"`
	Latest            string                    `json:"latest,omitempty"`
	Status            string                    `json:"status"`
	Command           string                    `json:"command,omitempty"`
	Action            string                    `json:"action,omitempty"`
	Rollback          string                    `json:"rollback,omitempty"`
	RestartClients    []string                  `json:"restart_clients,omitempty"`
	Stages            []UpgradeStage            `json:"stages"`
	ConfiguredClients []UpgradeConfiguredClient `json:"configured_clients,omitempty"`
}
