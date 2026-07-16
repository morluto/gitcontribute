package cli

import "context"

// bootstrapService is a placeholder that returns ErrNotWired for every method.
// It exists so the CLI can be built and run before real corpus, GitHub, MCP,
// or Gitcrawl integrations are wired in.
type bootstrapService struct{}

func (bootstrapService) Init(ctx context.Context) (*InitResult, error) {
	return nil, NewCLIError(ExitNotWired, ErrNotWired)
}

func (bootstrapService) Status(ctx context.Context) (*StatusResult, error) {
	return nil, NewCLIError(ExitNotWired, ErrNotWired)
}

func (bootstrapService) Sync(ctx context.Context, repo RepoRef) (*SyncResult, error) {
	return nil, NewCLIError(ExitNotWired, ErrNotWired)
}

func (bootstrapService) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	return nil, NewCLIError(ExitNotWired, ErrNotWired)
}

func (bootstrapService) Dossier(ctx context.Context, repo RepoRef) (*DossierResult, error) {
	return nil, NewCLIError(ExitNotWired, ErrNotWired)
}

// bootstrapMCPRunner is a placeholder MCP runner that returns ErrNotWired.
type bootstrapMCPRunner struct{}

func (bootstrapMCPRunner) Run(ctx context.Context, opts MCPOptions) error {
	return NewCLIError(ExitNotWired, ErrNotWired)
}

// NewBootstrapService returns a Service that reports ErrNotWired.
func NewBootstrapService() Service { return bootstrapService{} }

// NewBootstrapMCPRunner returns an MCPRunner that reports ErrNotWired.
func NewBootstrapMCPRunner() MCPRunner { return bootstrapMCPRunner{} }
