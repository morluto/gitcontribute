package cli

import "context"

// SetupService exposes local onboarding and client-registration capabilities.
// Setup may write local configuration, initialize the corpus, and explicitly
// invoke npm to install the terminal app. It must not perform GitHub network
// access or execute repository-controlled code.
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
	// SetupPhaseTerminal installs and verifies the persistent terminal command.
	SetupPhaseTerminal SetupPhase = "terminal"
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

// SetupOptions selects independent onboarding capabilities. InstallCLI controls
// the package-manager mutation; Clients and AllClients control MCP registration.
// SkipMCP permits terminal-only setup and is mutually exclusive with client
// selections. DryRun plans every selected capability without invoking npm or
// writing local state.
type SetupOptions struct {
	Remove     bool
	Clients    []string
	AllClients bool

	// InstallCLI explicitly authorizes a global npm installation of the running
	// GitContribute version. It is never inferred in non-interactive operation.
	InstallCLI bool
	// SkipMCP selects terminal-only setup. An empty Clients slice without
	// SkipMCP means detect installed clients rather than disable MCP.
	SkipMCP bool

	TokenSource    string
	TokenSourceKey string
	Repository     string
	DryRun         bool
	// Version is the release used for both persistent installation and an npx
	// MCP launcher. Empty values inherit the running service version.
	Version string
	// Executable and Environment are runtime evidence used to choose a stable
	// MCP launcher. They are injectable so setup behavior is testable without
	// capturing a temporary npm-cache executable.
	Executable  string
	Environment map[string]string
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

// SetupReport records the effects attempted by setup. Launcher is populated
// only when MCP was selected and contains the exact command registered with
// clients. A report may contain both successful and failed independent steps.
type SetupReport struct {
	Operation      string               `json:"operation"`
	DryRun         bool                 `json:"dry_run"`
	Launcher       string               `json:"launcher,omitempty"`
	Authentication *SetupAuthentication `json:"authentication,omitempty"`
	Steps          []SetupStep          `json:"steps"`
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
