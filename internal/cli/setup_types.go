package cli

import "context"

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

// InstallsCLI reports whether setup should install the global command.
func (m SetupMode) InstallsCLI() bool { return m == SetupModeCLI || m == SetupModeBoth }

// ConfiguresMCP reports whether setup should register coding-agent access.
func (m SetupMode) ConfiguresMCP() bool { return m == SetupModeMCP || m == SetupModeBoth }

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
	Operation         string               `json:"operation"`
	DryRun            bool                 `json:"dry_run"`
	MCPCommand        *SetupMCPCommand     `json:"mcp_command,omitempty"`
	MCPCommandPending bool                 `json:"mcp_command_pending,omitempty"`
	Authentication    *SetupAuthentication `json:"authentication,omitempty"`
	Steps             []SetupStep          `json:"steps"`
}

// HasFailures reports whether setup could not produce a usable result. A nil
// report is a failure because callers cannot verify any planned or applied step.
func (r *SetupReport) HasFailures() bool {
	if r == nil {
		return true
	}
	for _, step := range r.Steps {
		if step.Status == "failed" {
			return true
		}
	}
	return false
}
