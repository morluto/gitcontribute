package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SearchGitHubUsers performs one bounded live discovery page and persists only
// identity observations; it never expands the result into N profile reads.
func (r *MCPReader) SearchGitHubUsers(ctx context.Context, in mcpcontract.SearchGitHubUsersInput) (mcpcontract.SearchGitHubUsersOutput, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return mcpcontract.SearchGitHubUsersOutput{}, errors.New("query is required")
	}
	if in.Sort == "" {
		in.Sort = "best_match"
	}
	if in.Order == "" {
		in.Order = "desc"
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Limit < 1 || in.Limit > 100 || in.Page < 1 || in.Page > 10 {
		return mcpcontract.SearchGitHubUsersOutput{}, errors.New("limit must be 1 to 100 and page must be 1 to 10")
	}
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpcontract.SearchGitHubUsersOutput{}, err
	}
	searcher, ok := reader.(github.UserSearcher)
	if !ok {
		return mcpcontract.SearchGitHubUsersOutput{}, errors.New("GitHub user search is not available")
	}
	result, err := searcher.SearchUsers(ctx, github.UserSearchOptions{Query: in.Query, Sort: mapBestMatch(in.Sort), Order: in.Order, Page: in.Page, PerPage: in.Limit})
	if err != nil {
		return mcpcontract.SearchGitHubUsersOutput{}, err
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchGitHubUsersOutput{}, err
	}
	observedAt := r.now().UTC()
	out := mcpcontract.SearchGitHubUsersOutput{Query: in.Query, Total: result.Total, Incomplete: result.Incomplete, Page: in.Page, ObservedAt: formatTime(observedAt), Rate: githubRateOutput(result.Rate), Items: make([]mcpcontract.ActorIdentityOutput, 0, len(result.Items))}
	if result.Page.NextPage > 0 {
		out.NextPage = result.Page.NextPage
	}
	for _, actor := range result.Items {
		payload, _ := json.Marshal(actor)
		databaseID := actor.ID
		stored, err := c.ApplyActorIdentityObservation(ctx, "github", actor.Login, actor.NodeID, &databaseID, normalizeActorKind(actor.Kind), "public", observedAt, payload)
		if err != nil {
			return mcpcontract.SearchGitHubUsersOutput{}, err
		}
		out.Items = append(out.Items, actorIdentityOutput(stored))
	}
	return out, nil
}

func mapBestMatch(sort string) string {
	if sort == "best_match" {
		return ""
	}
	return sort
}

// SyncUsers submits a durable exact-profile acquisition batch.
func (r *MCPReader) SyncUsers(ctx context.Context, in mcpcontract.SyncUsersInput) (mcpcontract.JobReference, error) {
	if len(in.Users) < 1 || len(in.Users) > 100 {
		return mcpcontract.JobReference{}, errors.New("users must contain 1 to 100 items")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = len(in.Users)
	}
	if in.MaxRequests < len(in.Users) || in.MaxRequests > 100 {
		return mcpcontract.JobReference{}, errors.New("max_requests must admit every user and cannot exceed 100")
	}
	if err := validateActorSelectors(in.Users); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "sync_users", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncUsers(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_users", "GitHub user profile synchronization started"), nil
}

func (r *MCPReader) syncUsers(ctx context.Context, in mcpcontract.SyncUsersInput, report func(string, string) error) (map[string]any, error) {
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return nil, err
	}
	profiles, ok := reader.(github.UserProfileReader)
	if !ok {
		return nil, errors.New("GitHub user profile reads are not available")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, len(in.Users))
	complete := 0
	if err := report("profiles", jobProgressCounts(0, len(in.Users))); err != nil {
		return nil, err
	}
	for index, selector := range in.Users {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		login, resolveErr := resolveActorSelectorLogin(ctx, c, selector)
		if resolveErr != nil {
			items[index] = map[string]any{"key": actorSelectorKey(selector), "status": "unavailable", "reason": "actor_login_unknown", "message": resolveErr.Error()}
			if err := report("profiles", jobProgressCounts(index+1, len(in.Users))); err != nil {
				return nil, err
			}
			continue
		}
		actor, _, readErr := profiles.GetUser(ctx, login)
		if readErr != nil {
			itemStatus, reason, message, retry := githubBatchError(readErr)
			items[index] = map[string]any{"key": actorSelectorKey(selector), "status": itemStatus, "reason": reason, "message": message, "retry_after_ms": retry}
			if err := report("profiles", jobProgressCounts(index+1, len(in.Users))); err != nil {
				return nil, err
			}
			continue
		}
		payload, _ := json.Marshal(actor)
		databaseID := actor.ID
		stored, persistErr := c.ApplyActorProfileObservation(ctx, corpus.ActorProfileObservation{
			Provider: "github", NodeID: actor.NodeID, DatabaseID: &databaseID, Kind: normalizeActorKind(actor.Kind), Login: actor.Login,
			SourceUpdatedAt: actor.UpdatedAt, ObservedAt: r.now().UTC(), AuthorizationScope: "public", RawPayload: payload,
			Profile: corpus.ActorProfile{Name: actor.Name, AvatarURL: actor.AvatarURL, Bio: actor.Bio, Company: actor.Company, Location: actor.Location, WebsiteURL: actor.WebsiteURL, PublicEmail: actor.PublicEmail, TwitterUsername: actor.TwitterUsername, Hireable: actor.Hireable, Followers: actor.Followers, Following: actor.Following, PublicRepositories: actor.PublicRepositories, PublicGists: actor.PublicGists, ProviderCreatedAt: actor.CreatedAt},
		})
		if persistErr != nil {
			return nil, persistErr
		}
		items[index] = map[string]any{"key": actorSelectorKey(selector), "status": "complete", "actor_id": stored.Key, "login": stored.Login}
		complete++
		if err := report("profiles", jobProgressCounts(index+1, len(in.Users))); err != nil {
			return nil, err
		}
	}
	status := "complete"
	if complete != len(in.Users) {
		status = "partial"
	}
	return map[string]any{"status": status, "items": items, "completed": complete, "total": len(in.Users)}, nil
}

func validateActorSelectors(selectors []mcpcontract.ActorSelector) error {
	seen := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		key := actorSelectorKey(selector)
		switch selector.Type {
		case "login":
			if strings.TrimSpace(selector.Login) == "" || selector.NodeID != "" {
				return errors.New("login selectors require login and forbid node_id")
			}
		case "node_id":
			if strings.TrimSpace(selector.NodeID) == "" || selector.Login != "" {
				return errors.New("node_id selectors require node_id and forbid login")
			}
		default:
			return errors.New("actor selector type must be login or node_id")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate actor selector %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func actorSelectorKey(selector mcpcontract.ActorSelector) string {
	if selector.Type == "node_id" {
		return strings.TrimSpace(selector.NodeID)
	}
	return strings.ToLower(strings.TrimSpace(selector.Login))
}

func resolveActorSelectorLogin(ctx context.Context, c *corpus.Corpus, selector mcpcontract.ActorSelector) (string, error) {
	if selector.Type == "login" {
		return strings.TrimSpace(selector.Login), nil
	}
	nodeID := strings.TrimSpace(selector.NodeID)
	actor, err := c.GetActor(ctx, nodeID)
	if err != nil {
		return "", err
	}
	if actor == nil || actor.Login == "" {
		return "", fmt.Errorf("node ID %q is not stored; search or sync by login first", nodeID)
	}
	return actor.Login, nil
}

func normalizeActorKind(kind string) string {
	switch strings.ToLower(kind) {
	case "user", "bot", "organization", "mannequin":
		return strings.ToLower(kind)
	default:
		return "unknown"
	}
}

// SearchActors is a local-only actor search.
func (r *MCPReader) SearchActors(ctx context.Context, in mcpcontract.SearchActorsInput) (mcpcontract.SearchActorsOutput, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchActorsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.SearchActorsOutput{}, err
	}
	page, err := c.SearchActors(ctx, corpus.ActorSearchOptions{Query: in.Query, Kinds: in.Kinds, Sort: in.Sort, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return mcpcontract.SearchActorsOutput{}, err
	}
	out := mcpcontract.SearchActorsOutput{Items: make([]mcpcontract.ActorOutput, 0, len(page.Actors)), Total: page.Total, NextCursor: page.NextCursor, SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	for _, actor := range page.Actors {
		out.Items = append(out.Items, actorOutput(actor))
	}
	return out, nil
}

// GetActors performs an input-ordered local actor read.
func (r *MCPReader) GetActors(ctx context.Context, in mcpcontract.GetActorsInput) (mcpcontract.GetActorsOutput, error) {
	if len(in.Actors) < 1 || len(in.Actors) > 100 {
		return mcpcontract.GetActorsOutput{}, errors.New("actors must contain 1 to 100 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetActorsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetActorsOutput{}, err
	}
	out := mcpcontract.GetActorsOutput{Items: make([]mcpcontract.ActorBatchItem[mcpcontract.ActorOutput], len(in.Actors)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	for index, ref := range in.Actors {
		item := mcpcontract.ActorBatchItem[mcpcontract.ActorOutput]{Key: ref, Status: "complete"}
		actor, readErr := c.GetActor(ctx, ref)
		if readErr != nil {
			item.Status = "failed"
			item.Reason = "actor_read_failed"
			item.Message = readErr.Error()
		} else if actor == nil {
			item.Status = "unavailable"
			item.Reason = "actor_not_indexed"
			item.Message = "actor is not present in the local corpus"
		} else {
			value := actorOutput(*actor)
			item.Value = &value
		}
		out.Items[index] = item
	}
	return out, nil
}

// GetActorFacets reads local facet coverage and returns opaque resource URIs.
func (r *MCPReader) GetActorFacets(ctx context.Context, in mcpcontract.GetActorFacetsInput) (mcpcontract.GetActorFacetsOutput, error) {
	if len(in.Actors) < 1 || len(in.Actors) > 100 {
		return mcpcontract.GetActorFacetsOutput{}, errors.New("actors must contain 1 to 100 items")
	}
	if len(in.Facets) < 1 || len(in.Facets) > 7 {
		return mcpcontract.GetActorFacetsOutput{}, errors.New("facets must contain 1 to 7 non-period facets")
	}
	valid := map[string]bool{"profile": true, "social_accounts": true, "organizations": true, "pinned_items": true, "repositories:owned": true, "repositories:affiliated": true, "repositories:contributed": true}
	for _, facet := range in.Facets {
		if !valid[facet] {
			return mcpcontract.GetActorFacetsOutput{}, fmt.Errorf("unsupported actor facet %q", facet)
		}
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetActorFacetsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetActorFacetsOutput{}, err
	}
	out := mcpcontract.GetActorFacetsOutput{Items: make([]mcpcontract.ActorBatchItem[mcpcontract.ActorFacetReferenceOutput], len(in.Actors)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	for index, ref := range in.Actors {
		item := mcpcontract.ActorBatchItem[mcpcontract.ActorFacetReferenceOutput]{Key: ref, Status: "complete"}
		actor, readErr := c.GetActor(ctx, ref)
		if readErr != nil {
			item.Status = "failed"
			item.Reason = "actor_read_failed"
			item.Message = readErr.Error()
			out.Items[index] = item
			continue
		}
		if actor == nil {
			item.Status = "unavailable"
			item.Reason = "actor_not_indexed"
			item.Message = "actor is not present in the local corpus"
			out.Items[index] = item
			continue
		}
		coverageByFacet, coverageErr := c.ListActorFacetCoverage(ctx, actor.ID, in.Facets)
		if coverageErr != nil {
			return mcpcontract.GetActorFacetsOutput{}, coverageErr
		}
		value := mcpcontract.ActorFacetReferenceOutput{ActorID: actor.Key, Facets: make([]mcpcontract.ActorCoverageOutput, 0, len(in.Facets)), URIs: make([]string, 0, len(in.Facets))}
		for _, facet := range in.Facets {
			coverage := mcpcontract.ActorCoverageOutput{Facet: facet, Status: "unknown", Reason: "facet_not_synchronized"}
			if stored, ok := coverageByFacet[facet]; ok {
				coverage.Status = map[bool]string{true: "complete", false: "truncated"}[stored.Complete]
				coverage.ObservedAt = formatTime(stored.ObservedAt)
				coverage.SourceUpdatedAt = formatTime(stored.SourceUpdatedAt)
				coverage.AuthorizationScope = stored.AuthorizationScope
				if stored.Complete {
					coverage.Reason = ""
				} else {
					coverage.Truncated = true
					coverage.Reason = "facet_incomplete"
				}
			}
			value.Facets = append(value.Facets, coverage)
			value.URIs = append(value.URIs, "gitcontribute://actor/"+url.PathEscape(actor.Key)+"/facet/"+url.PathEscape(facet))
			if coverage.Status != "complete" {
				item.Status = "unavailable"
				item.Reason = "actor_facet_unknown"
				item.Message = "one or more requested actor facets are not completely synchronized"
			}
		}
		item.Value = &value
		out.Items[index] = item
	}
	return out, nil
}

// ActorResource returns the canonical stored actor view without refreshing it.
func (r *MCPReader) ActorResource(ctx context.Context, ref, facet string) (any, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	actor, err := c.GetActor(ctx, ref)
	if err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, mcpcontract.ErrNotFound
	}
	if facet == "" || facet == "profile" {
		return actorOutput(*actor), nil
	}
	observation, err := c.GetActorFacetObservation(ctx, actor.ID, facet)
	if err != nil {
		return nil, err
	}
	if observation == nil {
		return nil, mcpcontract.ErrNotFound
	}
	var value any
	if err := json.Unmarshal(observation.Payload, &value); err != nil {
		value = string(observation.Payload)
	}
	return map[string]any{"schema_version": "gitcontribute.actor-facet.v1", "actor_id": actor.Key, "facet": facet, "complete": observation.Complete, "observed_at": formatTime(observation.ObservedAt), "source_updated_at": formatTime(observation.SourceUpdatedAt), "authorization_scope": observation.AuthorizationScope, "value": value}, nil
}

func actorIdentityOutput(actor corpus.Actor) mcpcontract.ActorIdentityOutput {
	return mcpcontract.ActorIdentityOutput{ActorID: actor.Key, Provider: actor.Provider, NodeID: actor.NodeID, DatabaseID: actor.DatabaseID, Kind: actor.Kind, Login: actor.Login}
}

func actorOutput(actor corpus.Actor) mcpcontract.ActorOutput {
	out := mcpcontract.ActorOutput{ActorIdentityOutput: actorIdentityOutput(actor), URI: "gitcontribute://actor/" + url.PathEscape(actor.Key)}
	coverage := mcpcontract.ActorCoverageOutput{Facet: "profile", Status: "unknown", Reason: "profile_not_synchronized"}
	if actor.Profile != nil {
		p := actor.Profile
		out.Profile = &mcpcontract.ActorProfileOutput{Name: p.Name, AvatarURL: p.AvatarURL, Bio: p.Bio, Company: p.Company, Location: p.Location, WebsiteURL: p.WebsiteURL, PublicEmail: p.PublicEmail, TwitterUsername: p.TwitterUsername, Hireable: p.Hireable, Followers: p.Followers, Following: p.Following, PublicRepositories: p.PublicRepositories, PublicGists: p.PublicGists, ProviderCreatedAt: formatTime(p.ProviderCreatedAt)}
		coverage.Status = "complete"
		coverage.Reason = ""
		coverage.ObservedAt = formatTime(p.ObservedAt)
		coverage.SourceUpdatedAt = formatTime(p.SourceUpdatedAt)
		coverage.AuthorizationScope = p.AuthorizationScope
	}
	out.Coverage = []mcpcontract.ActorCoverageOutput{coverage}
	return out
}
