package cli

import "context"

// SetupService exposes local onboarding and client-registration capabilities.
// Setup may write local configuration, initialize the corpus, and explicitly
// invoke npm to install the terminal app. It must not perform GitHub network
// access or execute repository-controlled code.
type SetupService interface {
	Setup(ctx context.Context, opts SetupOptions) (*SetupReport, error)
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

// SetupReport records the effects attempted by setup. Launcher is populated
// only when MCP was selected and contains the exact command registered with
// clients. A report may contain both successful and failed independent steps.
type SetupReport struct {
	Operation string      `json:"operation"`
	DryRun    bool        `json:"dry_run"`
	Launcher  string      `json:"launcher,omitempty"`
	Steps     []SetupStep `json:"steps"`
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
