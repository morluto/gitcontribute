package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) registerActorTools() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s, catalogTool[mcpcontract.SearchActorsInput, mcpcontract.SearchActorsOutput]{
		name: mcpcontract.ToolSearchActors, title: "Search stored GitHub actors",
		description: "Search indexed GitHub users, bots, organizations, and other actors by stored profile facts. Offline; returns snapshot-bound facts and coverage.",
		annotations: readOnly, supportedBy: supports[ActorReader], input: inputSchema[mcpcontract.SearchActorsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "kinds", 1, 5)
			setArrayEnum(sc, "kinds", "user", "bot", "organization", "mannequin", "unknown")
			setEnum(sc, "sort", "relevance", "login", "followers", "public_repositories", "profile_updated_at", "observed_at")
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}),
		output: outputSchema[mcpcontract.SearchActorsOutput]("Snapshot-bound stored actor search results."), handler: s.searchActors,
	})
	addCatalogTool(s, catalogTool[mcpcontract.GetActorsInput, mcpcontract.GetActorsOutput]{
		name: mcpcontract.ToolGetActors, title: "Get stored GitHub actors",
		description: "Read exact stored actor identities, nullable profile facts, and profile coverage for up to 100 references. Offline.",
		annotations: readOnly, supportedBy: supports[ActorReader], input: inputSchema[mcpcontract.GetActorsInput](func(sc *schemaBuilder) { setArrayBounds(sc, "actors", 1, 100) }),
		output: outputSchema[mcpcontract.GetActorsOutput]("Ordered exact actor results with item-level availability."), handler: s.getActors,
	})
	addCatalogTool(s, catalogTool[mcpcontract.GetActorFacetsInput, mcpcontract.GetActorFacetsOutput]{
		name: mcpcontract.ToolGetActorFacets, title: "Get stored actor facet coverage",
		description: "Read coverage and canonical resource URIs for selected stored actor facets. Offline; missing observations remain unknown.",
		annotations: readOnly, supportedBy: supports[ActorReader], input: inputSchema[mcpcontract.GetActorFacetsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "actors", 1, 100)
			setArrayBounds(sc, "facets", 1, 7)
			setArrayEnum(sc, "facets", "profile", "social_accounts", "organizations", "pinned_items", "repositories:owned", "repositories:affiliated", "repositories:contributed")
		}),
		output: outputSchema[mcpcontract.GetActorFacetsOutput]("Ordered actor facet coverage and resource references."), handler: s.getActorFacets,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SearchGitHubUsersInput, mcpcontract.SearchGitHubUsersOutput]{
		name: mcpcontract.ToolSearchGitHubUsers, title: "Search live GitHub users",
		description: "Search one bounded GitHub user page and persist identity observations. It does not hydrate result profiles.",
		annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator], input: inputSchema[mcpcontract.SearchGitHubUsersInput](func(sc *schemaBuilder) {
			setEnum(sc, "sort", "best_match", "followers", "repositories", "joined")
			setDefault(sc, "sort", "best_match")
			setEnum(sc, "order", "asc", "desc")
			setDefault(sc, "order", "desc")
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
			setRange(sc, "page", 1, 10)
			setDefault(sc, "page", 1)
		}),
		output: outputSchema[mcpcontract.SearchGitHubUsersOutput]("One persisted live GitHub user-search page."), handler: s.searchGitHubUsers,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SyncUsersInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolSyncUsers, title: "Sync exact GitHub user profiles",
		description: "Fetch and persist profile headers for an ordered bounded set of exact GitHub users. Returns a durable job reference.",
		annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator], input: inputSchema[mcpcontract.SyncUsersInput](func(sc *schemaBuilder) {
			configureActorSelectorModes(sc)
			setArrayBounds(sc, "users", 1, 100)
			setRange(sc, "max_requests", 1, 100)
		}),
		output: outputSchema[mcpcontract.JobReference]("Durable exact-user profile synchronization job."), handler: s.syncUsers,
	})
	registerFacet := func(name, title, description string, handler mcp.ToolHandlerFor[mcpcontract.SyncUserFacetInput, mcpcontract.JobReference]) {
		addCatalogTool(s, catalogTool[mcpcontract.SyncUserFacetInput, mcpcontract.JobReference]{
			name: name, title: title, description: description,
			annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator], input: inputSchema[mcpcontract.SyncUserFacetInput](func(sc *schemaBuilder) {
				configureActorSelectorModes(sc)
				setArrayBounds(sc, "users", 1, 100)
				setRange(sc, "max_pages", 1, 10)
				setDefault(sc, "max_pages", 1)
				setRange(sc, "max_items_per_user", 1, 1000)
				setDefault(sc, "max_items_per_user", 100)
				setRange(sc, "max_requests", 1, 1000)
			}), output: outputSchema[mcpcontract.JobReference]("Durable bounded actor-facet synchronization job."), handler: handler,
		})
	}
	registerFacet(mcpcontract.ToolSyncUserSocialAccounts, "Sync GitHub user social accounts", "Fetch and replace bounded public social-account facts for exact stored users.", s.syncUserSocialAccounts)
	registerFacet(mcpcontract.ToolSyncUserOrganizations, "Sync GitHub user organizations", "Fetch and replace bounded public organization memberships for exact stored users.", s.syncUserOrganizations)
	addCatalogTool(s, catalogTool[mcpcontract.SyncUserPinnedItemsInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolSyncUserPinnedItems, title: "Sync GitHub user pinned items",
		description: "Fetch and replace the public pinned or profile-showcase items for exact stored users.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator],
		input: inputSchema[mcpcontract.SyncUserPinnedItemsInput](func(sc *schemaBuilder) {
			configureActorSelectorModes(sc)
			setArrayBounds(sc, "users", 1, 50)
			setRange(sc, "limit", 1, 6)
			setDefault(sc, "limit", 6)
			setRange(sc, "max_requests", 1, 100)
		}),
		output: outputSchema[mcpcontract.JobReference]("Durable pinned-item synchronization job."), handler: s.syncUserPinnedItems,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SyncUserRepositoriesInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolSyncUserRepositories, title: "Sync GitHub user repositories",
		description: "Fetch and replace one explicit repository relationship for exact stored users: owned, affiliated, or contributed.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator],
		input: inputSchema[mcpcontract.SyncUserRepositoriesInput](func(sc *schemaBuilder) {
			configureActorSelectorModes(sc)
			setArrayBounds(sc, "users", 1, 50)
			setEnum(sc, "relationship", "owned", "affiliated", "contributed")
			setEnum(sc, "sort", "created", "updated", "pushed", "full_name")
			setEnum(sc, "order", "asc", "desc")
			setRange(sc, "max_pages", 1, 10)
			setDefault(sc, "max_pages", 1)
			setRange(sc, "max_items_per_user", 1, 1000)
			setDefault(sc, "max_items_per_user", 100)
			setRange(sc, "max_requests", 1, 1000)
		}), output: outputSchema[mcpcontract.JobReference]("Durable repository-relationship synchronization job."), handler: s.syncUserRepositories,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SyncUserContributionsInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolSyncUserContributions, title: "Sync GitHub user contributions",
		description: "Fetch one explicit contribution period for exact stored users. Restricted contributions remain aggregate facts.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubActorOperator],
		input: inputSchema[mcpcontract.SyncUserContributionsInput](func(sc *schemaBuilder) {
			configureActorSelectorModes(sc)
			setArrayBounds(sc, "users", 1, 20)
			setRange(sc, "max_repositories", 1, 100)
			setDefault(sc, "max_repositories", 25)
			setRange(sc, "max_requests", 1, 100)
		}),
		output: outputSchema[mcpcontract.JobReference]("Durable bounded contribution-period synchronization job."), handler: s.syncUserContributions,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SearchContributionsInput, mcpcontract.SearchContributionsOutput]{
		name: mcpcontract.ToolSearchContributions, title: "Search stored actor contributions",
		description: "Search stored GitHub-profile contribution facts by actor, repository, kind, and time. Offline; returns explicit coverage.", annotations: readOnly, supportedBy: supports[ActorReader],
		input: inputSchema[mcpcontract.SearchContributionsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "actors", 1, 100)
			setArrayBounds(sc, "repositories", 1, 100)
			setArrayBounds(sc, "kinds", 1, 20)
			setEnum(sc, "source", "github_profile")
			setDefault(sc, "source", "github_profile")
			setEnum(sc, "sort", "occurred_at", "repository", "type")
			setDefault(sc, "sort", "occurred_at")
			setEnum(sc, "order", "asc", "desc")
			setDefault(sc, "order", "desc")
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}),
		output: outputSchema[mcpcontract.SearchContributionsOutput]("Snapshot-bound contribution facts and acquisition coverage."), handler: s.searchContributions,
	})
}

func (s *Server) searchActors(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchActorsInput) (*mcp.CallToolResult, mcpcontract.SearchActorsOutput, error) {
	reader, ok := s.reader.(ActorReader)
	if !ok {
		return nil, mcpcontract.SearchActorsOutput{}, errors.New("actor search is not available")
	}
	out, err := reader.SearchActors(ctx, in)
	return nil, out, err
}
func (s *Server) getActors(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetActorsInput) (*mcp.CallToolResult, mcpcontract.GetActorsOutput, error) {
	reader, ok := s.reader.(ActorReader)
	if !ok {
		return nil, mcpcontract.GetActorsOutput{}, errors.New("actor reads are not available")
	}
	out, err := reader.GetActors(ctx, in)
	return nil, out, err
}
func (s *Server) getActorFacets(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetActorFacetsInput) (*mcp.CallToolResult, mcpcontract.GetActorFacetsOutput, error) {
	reader, ok := s.reader.(ActorReader)
	if !ok {
		return nil, mcpcontract.GetActorFacetsOutput{}, errors.New("actor facets are not available")
	}
	out, err := reader.GetActorFacets(ctx, in)
	return nil, out, err
}
func (s *Server) searchGitHubUsers(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchGitHubUsersInput) (*mcp.CallToolResult, mcpcontract.SearchGitHubUsersOutput, error) {
	reader, ok := s.reader.(GitHubActorOperator)
	if !ok {
		return nil, mcpcontract.SearchGitHubUsersOutput{}, errors.New("GitHub user search is not available")
	}
	out, err := reader.SearchGitHubUsers(ctx, in)
	return nil, out, err
}
func (s *Server) syncUsers(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUsersInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	reader, ok := s.reader.(GitHubActorOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("GitHub user synchronization is not available")
	}
	out, err := reader.SyncUsers(ctx, in)
	return nil, out, err
}

func (s *Server) syncUserSocialAccounts(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUserFacetInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	return actorJobCall(ctx, s, func(operator GitHubActorOperator) (mcpcontract.JobReference, error) {
		return operator.SyncUserSocialAccounts(ctx, in)
	})
}
func (s *Server) syncUserOrganizations(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUserFacetInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	return actorJobCall(ctx, s, func(operator GitHubActorOperator) (mcpcontract.JobReference, error) {
		return operator.SyncUserOrganizations(ctx, in)
	})
}
func (s *Server) syncUserPinnedItems(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUserPinnedItemsInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	return actorJobCall(ctx, s, func(operator GitHubActorOperator) (mcpcontract.JobReference, error) {
		return operator.SyncUserPinnedItems(ctx, in)
	})
}
func (s *Server) syncUserRepositories(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUserRepositoriesInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	return actorJobCall(ctx, s, func(operator GitHubActorOperator) (mcpcontract.JobReference, error) {
		return operator.SyncUserRepositories(ctx, in)
	})
}
func (s *Server) syncUserContributions(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncUserContributionsInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	return actorJobCall(ctx, s, func(operator GitHubActorOperator) (mcpcontract.JobReference, error) {
		return operator.SyncUserContributions(ctx, in)
	})
}
func actorJobCall[T any](_ context.Context, s *Server, call func(GitHubActorOperator) (T, error)) (*mcp.CallToolResult, T, error) {
	operator, ok := s.reader.(GitHubActorOperator)
	if !ok {
		var zero T
		return nil, zero, errors.New("GitHub actor synchronization is not available")
	}
	out, err := call(operator)
	return nil, out, err
}
func (s *Server) searchContributions(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchContributionsInput) (*mcp.CallToolResult, mcpcontract.SearchContributionsOutput, error) {
	reader, ok := s.reader.(ActorReader)
	if !ok {
		return nil, mcpcontract.SearchContributionsOutput{}, errors.New("actor contribution search is not available")
	}
	out, err := reader.SearchContributions(ctx, in)
	return nil, out, err
}
