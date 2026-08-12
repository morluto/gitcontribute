package mcpserver

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type actorCapabilityReader struct{ mcpcontract.Reader }

func (actorCapabilityReader) SearchGitHubUsers(context.Context, mcpcontract.SearchGitHubUsersInput) (mcpcontract.SearchGitHubUsersOutput, error) {
	return mcpcontract.SearchGitHubUsersOutput{}, nil
}
func (actorCapabilityReader) SyncUsers(context.Context, mcpcontract.SyncUsersInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SyncUserSocialAccounts(context.Context, mcpcontract.SyncUserFacetInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SyncUserOrganizations(context.Context, mcpcontract.SyncUserFacetInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SyncUserPinnedItems(context.Context, mcpcontract.SyncUserPinnedItemsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SyncUserRepositories(context.Context, mcpcontract.SyncUserRepositoriesInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SyncUserContributions(context.Context, mcpcontract.SyncUserContributionsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{}, nil
}
func (actorCapabilityReader) SearchActors(context.Context, mcpcontract.SearchActorsInput) (mcpcontract.SearchActorsOutput, error) {
	return mcpcontract.SearchActorsOutput{}, nil
}
func (actorCapabilityReader) GetActors(context.Context, mcpcontract.GetActorsInput) (mcpcontract.GetActorsOutput, error) {
	return mcpcontract.GetActorsOutput{}, nil
}
func (actorCapabilityReader) GetActorFacets(context.Context, mcpcontract.GetActorFacetsInput) (mcpcontract.GetActorFacetsOutput, error) {
	return mcpcontract.GetActorFacetsOutput{}, nil
}
func (actorCapabilityReader) SearchContributions(context.Context, mcpcontract.SearchContributionsInput) (mcpcontract.SearchContributionsOutput, error) {
	return mcpcontract.SearchContributionsOutput{}, nil
}

func TestActorCapabilitiesAdvertiseAtomicTools(t *testing.T) {
	base := &fakeReader{searchStarted: make(chan struct{})}
	tools, closeSessions := listedToolsFromReader(t, actorCapabilityReader{Reader: base})
	defer closeSessions()

	for _, name := range []string{
		mcpcontract.ToolSearchGitHubUsers, mcpcontract.ToolSyncUsers,
		mcpcontract.ToolSyncUserSocialAccounts, mcpcontract.ToolSyncUserOrganizations,
		mcpcontract.ToolSyncUserPinnedItems, mcpcontract.ToolSyncUserRepositories,
		mcpcontract.ToolSyncUserContributions, mcpcontract.ToolSearchActors,
		mcpcontract.ToolGetActors, mcpcontract.ToolGetActorFacets,
		mcpcontract.ToolSearchContributions,
	} {
		if tools[name] == nil {
			t.Errorf("atomic actor tool %q was not advertised", name)
		}
	}
}

func TestSearchGitHubUsersRejectsWhitespaceOnlyQuery(t *testing.T) {
	server := &Server{reader: actorCapabilityReader{Reader: &fakeReader{}}}
	_, _, err := server.searchGitHubUsers(context.Background(), nil, mcpcontract.SearchGitHubUsersInput{Query: " \t "})
	if err == nil {
		t.Fatal("whitespace-only GitHub user search query was accepted")
	}
}
