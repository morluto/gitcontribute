package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (r *MCPReader) SyncUserSocialAccounts(ctx context.Context, in mcpcontract.SyncUserFacetInput) (mcpcontract.JobReference, error) {
	return r.submitUserFacetJob(ctx, "sync_user_social_accounts", in, func(ctx context.Context, c *corpus.Corpus, reader github.Reader, selector mcpcontract.ActorSelector) (map[string]any, error) {
		source, ok := reader.(github.UserSocialAccountReader)
		if !ok {
			return nil, errors.New("GitHub social-account reads are unavailable")
		}
		actor, login, err := storedActorForSelector(ctx, c, selector)
		if err != nil {
			return nil, err
		}
		items := []corpus.ActorSocialAccount{}
		page := 1
		complete := false
		for page <= facetMaxPages(in.MaxPages) {
			result, err := source.ListUserSocialAccounts(ctx, login, github.PageOptions{Page: page, PerPage: min(100, facetMaxItems(in.MaxItems)-len(items))})
			if err != nil {
				return nil, err
			}
			for _, item := range result.Items {
				items = append(items, corpus.ActorSocialAccount{Provider: item.Provider, URL: item.URL, DisplayName: item.DisplayName})
			}
			if !result.Page.HasNext || len(items) >= facetMaxItems(in.MaxItems) {
				complete = !result.Page.HasNext
				break
			}
			page = result.Page.NextPage
		}
		raw, _ := json.Marshal(items)
		observed := r.now().UTC()
		if err := c.ReplaceActorSocialAccounts(ctx, actor.ID, items, complete, observed, observed, "public", raw); err != nil {
			return nil, err
		}
		return map[string]any{"actor_id": actor.Key, "login": login, "items": len(items), "complete": complete}, nil
	})
}

func (r *MCPReader) SyncUserOrganizations(ctx context.Context, in mcpcontract.SyncUserFacetInput) (mcpcontract.JobReference, error) {
	return r.submitUserFacetJob(ctx, "sync_user_organizations", in, func(ctx context.Context, c *corpus.Corpus, reader github.Reader, selector mcpcontract.ActorSelector) (map[string]any, error) {
		source, ok := reader.(github.UserOrganizationReader)
		if !ok {
			return nil, errors.New("GitHub organization reads are unavailable")
		}
		actor, login, err := storedActorForSelector(ctx, c, selector)
		if err != nil {
			return nil, err
		}
		items := []corpus.ActorOrganization{}
		cursor := ""
		complete := false
		for page := 0; page < facetMaxPages(in.MaxPages); page++ {
			result, err := source.ListUserOrganizations(ctx, login, github.CursorPageOptions{First: min(100, facetMaxItems(in.MaxItems)-len(items)), After: cursor})
			if err != nil {
				return nil, err
			}
			for _, item := range result.Items {
				items = append(items, corpus.ActorOrganization{NodeID: item.NodeID, Login: item.Login})
			}
			if !result.Page.HasNext || len(items) >= facetMaxItems(in.MaxItems) {
				complete = !result.Page.HasNext
				break
			}
			cursor = result.Page.EndCursor
		}
		raw, _ := json.Marshal(items)
		observed := r.now().UTC()
		if err := c.ReplaceActorOrganizations(ctx, actor.ID, items, complete, observed, observed, "public", raw); err != nil {
			return nil, err
		}
		return map[string]any{"actor_id": actor.Key, "login": login, "items": len(items), "complete": complete}, nil
	})
}

func (r *MCPReader) SyncUserPinnedItems(ctx context.Context, in mcpcontract.SyncUserPinnedItemsInput) (mcpcontract.JobReference, error) {
	if len(in.Users) < 1 || len(in.Users) > 50 {
		return mcpcontract.JobReference{}, errors.New("users must contain 1 to 50 items")
	}
	if err := validateActorSelectors(in.Users); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if in.Limit == 0 {
		in.Limit = 6
	}
	if in.Limit < 1 || in.Limit > 6 {
		return mcpcontract.JobReference{}, errors.New("limit must be 1 to 6")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = len(in.Users)
	}
	if in.MaxRequests < len(in.Users) || in.MaxRequests > 100 {
		return mcpcontract.JobReference{}, errors.New("max_requests must admit every user and cannot exceed 100")
	}
	id, err := r.submitJob(ctx, "sync_user_pinned_items", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
		if err != nil {
			return nil, err
		}
		source, ok := reader.(github.UserPinnedItemReader)
		if !ok {
			return nil, errors.New("GitHub pinned-item reads are unavailable")
		}
		c, err := r.openCorpus(ctx)
		if err != nil {
			return nil, err
		}
		return r.runActorFacetItems(ctx, in.Users, "pinned_items", report, func(selector mcpcontract.ActorSelector) (map[string]any, error) {
			actor, login, err := storedActorForSelector(ctx, c, selector)
			if err != nil {
				return nil, err
			}
			result, err := source.GetUserPinnedItems(ctx, login, in.Limit)
			if err != nil {
				return nil, err
			}
			items := make([]corpus.ActorPinnedItem, len(result.Items))
			for i, item := range result.Items {
				items[i] = corpus.ActorPinnedItem{Rank: item.Rank, Kind: item.Kind, NodeID: item.NodeID, Name: item.Name, RepositoryOwner: item.RepositoryOwner, ShowcaseKind: result.ShowcaseKind}
			}
			raw, _ := json.Marshal(result)
			observed := r.now().UTC()
			if err := c.ReplaceActorPinnedItems(ctx, actor.ID, items, result.Coverage.Complete, observed, observed, "public", raw); err != nil {
				return nil, err
			}
			return map[string]any{"actor_id": actor.Key, "login": login, "items": len(items), "showcase_kind": result.ShowcaseKind, "complete": result.Coverage.Complete}, nil
		})
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_user_pinned_items", "GitHub pinned-item synchronization started"), nil
}

func (r *MCPReader) SyncUserRepositories(ctx context.Context, in mcpcontract.SyncUserRepositoriesInput) (mcpcontract.JobReference, error) {
	if len(in.Users) < 1 || len(in.Users) > 50 {
		return mcpcontract.JobReference{}, errors.New("users must contain 1 to 50 items")
	}
	if err := validateActorSelectors(in.Users); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if in.Relationship != "owned" && in.Relationship != "affiliated" && in.Relationship != "contributed" {
		return mcpcontract.JobReference{}, errors.New("relationship must be owned, affiliated, or contributed")
	}
	if err := normalizeFacetBounds(&in.MaxPages, &in.MaxItems, &in.MaxRequests, len(in.Users)); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "sync_user_repositories", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
		if err != nil {
			return nil, err
		}
		source, ok := reader.(github.UserRepositoryReader)
		if !ok {
			return nil, errors.New("GitHub user repository reads are unavailable")
		}
		c, err := r.openCorpus(ctx)
		if err != nil {
			return nil, err
		}
		return r.runActorFacetItems(ctx, in.Users, "repositories", report, func(selector mcpcontract.ActorSelector) (map[string]any, error) {
			actor, login, err := storedActorForSelector(ctx, c, selector)
			if err != nil {
				return nil, err
			}
			repositories := []github.Repository{}
			page, cursor := 1, ""
			complete := false
			for attempt := 0; attempt < in.MaxPages && len(repositories) < in.MaxItems; attempt++ {
				result, err := source.ListUserRepositories(ctx, login, github.UserRepositoryOptions{Relationship: in.Relationship, Sort: in.Sort, Direction: in.Order, After: cursor, PageOptions: github.PageOptions{Page: page, PerPage: min(100, in.MaxItems-len(repositories))}})
				if err != nil {
					return nil, err
				}
				repositories = append(repositories, result.Items...)
				if !result.Page.HasNext {
					complete = true
					break
				}
				page = result.Page.NextPage
				cursor = result.Page.EndCursor
			}
			affiliations := make([]corpus.ActorRepositoryAffiliation, 0, len(repositories))
			for _, remote := range repositories {
				payload, _ := json.Marshal(remote)
				stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
				if err != nil {
					return nil, err
				}
				if err := c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0); err != nil {
					return nil, err
				}
				affiliations = append(affiliations, corpus.ActorRepositoryAffiliation{RepositoryID: stored.ID, Relationship: in.Relationship})
			}
			raw, _ := json.Marshal(repositories)
			observed := r.now().UTC()
			if err := c.ReplaceActorRepositoryAffiliations(ctx, actor.ID, in.Relationship, affiliations, complete, observed, observed, "public", raw); err != nil {
				return nil, err
			}
			return map[string]any{"actor_id": actor.Key, "login": login, "items": len(repositories), "complete": complete, "relationship": in.Relationship}, nil
		})
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_user_repositories", "GitHub user repository synchronization started"), nil
}

// SyncUserContributions maps GitHub's typed contribution union into independently queryable corpus rows.
// Keeping the mapping together makes its atomic replacement boundary visible.
//
//nolint:gocognit
func (r *MCPReader) SyncUserContributions(ctx context.Context, in mcpcontract.SyncUserContributionsInput) (mcpcontract.JobReference, error) {
	if len(in.Users) < 1 || len(in.Users) > 20 {
		return mcpcontract.JobReference{}, errors.New("users must contain 1 to 20 items")
	}
	if err := validateActorSelectors(in.Users); err != nil {
		return mcpcontract.JobReference{}, err
	}
	from, err := time.Parse(time.RFC3339, in.From)
	if err != nil {
		return mcpcontract.JobReference{}, errors.New("from must be RFC 3339")
	}
	to, err := time.Parse(time.RFC3339, in.To)
	if err != nil {
		return mcpcontract.JobReference{}, errors.New("to must be RFC 3339")
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return mcpcontract.JobReference{}, errors.New("contribution period must be positive and no longer than one year")
	}
	if in.MaxRepositories == 0 {
		in.MaxRepositories = 25
	}
	if in.MaxRepositories < 1 || in.MaxRepositories > 100 {
		return mcpcontract.JobReference{}, errors.New("max_repositories must be 1 to 100")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = len(in.Users)
	}
	if in.MaxRequests < len(in.Users) || in.MaxRequests > 100 {
		return mcpcontract.JobReference{}, errors.New("max_requests must admit every user and cannot exceed 100")
	}
	id, err := r.submitJob(ctx, "sync_user_contributions", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
		if err != nil {
			return nil, err
		}
		source, ok := reader.(github.UserContributionReader)
		if !ok {
			return nil, errors.New("GitHub user contribution reads are unavailable")
		}
		c, err := r.openCorpus(ctx)
		if err != nil {
			return nil, err
		}
		return r.runActorFacetItems(ctx, in.Users, "contributions", report, func(selector mcpcontract.ActorSelector) (map[string]any, error) {
			actor, login, err := storedActorForSelector(ctx, c, selector)
			if err != nil {
				return nil, err
			}
			result, err := source.GetUserContributions(ctx, login, github.UserContributionOptions{From: from, To: to, OrganizationNodeID: in.OrganizationNodeID, MaxRepositories: in.MaxRepositories})
			if err != nil {
				return nil, err
			}
			repoIDs := make(map[string]int64)
			ensureRepo := func(nodeID, nameWithOwner string) (*int64, error) {
				if nodeID == "" || nameWithOwner == "" {
					return nil, nil
				}
				if id, ok := repoIDs[nodeID]; ok {
					return &id, nil
				}
				owner, name, ok := strings.Cut(nameWithOwner, "/")
				if !ok {
					return nil, nil
				}
				stored, err := c.GetRepository(ctx, owner, name)
				if err != nil {
					return nil, err
				}
				if stored == nil {
					stored, err = c.UpsertRepository(ctx, corpus.Repository{Owner: owner, Name: name, ExternalID: nodeID}, `{"source":"github-contributions"}`)
					if err != nil {
						return nil, err
					}
				}
				repoIDs[nodeID] = stored.ID
				id := stored.ID
				return &id, nil
			}
			items := make([]corpus.ActorContributionItem, 0, len(result.Items))
			for _, item := range result.Items {
				repositoryID, err := ensureRepo(item.RepositoryNodeID, item.RepositoryNameOwner)
				if err != nil {
					return nil, err
				}
				items = append(items, corpus.ActorContributionItem{Kind: item.Kind, OccurredAt: item.OccurredAt, RepositoryID: repositoryID, TargetNodeID: item.TargetNodeID, TargetURL: item.TargetURL, Restricted: item.Restricted, Count: item.Count})
			}
			totals := make([]corpus.ActorRepositoryContributionTotal, 0, len(result.RepositoryTotals))
			for _, total := range result.RepositoryTotals {
				repositoryID, err := ensureRepo(total.RepositoryNodeID, total.RepositoryNameOwner)
				if err != nil {
					return nil, err
				}
				if repositoryID != nil {
					totals = append(totals, corpus.ActorRepositoryContributionTotal{RepositoryID: *repositoryID, Kind: total.Kind, Count: total.Count})
				}
			}
			days := make([]corpus.ActorContributionDay, len(result.Days))
			for i, day := range result.Days {
				days[i] = corpus.ActorContributionDay{Date: day.Date, Count: day.Count, Level: day.Level}
			}
			raw, _ := json.Marshal(result)
			if err := c.ApplyActorContributionPeriod(ctx, corpus.ActorContributionPeriodInput{ActorID: actor.ID, From: from, To: to, OrganizationNodeID: in.OrganizationNodeID, AuthorizationScope: "viewer", TotalCommits: intPointer(result.TotalCommits), TotalIssues: intPointer(result.TotalIssues), TotalPullRequests: intPointer(result.TotalPullRequests), TotalPullRequestReviews: intPointer(result.TotalPullRequestReviews), TotalRepositories: intPointer(result.TotalRepositories), RestrictedContributions: intPointer(result.RestrictedContributions), Complete: result.Complete, ObservedAt: r.now().UTC(), SourceUpdatedAt: result.EndedAt, Days: days, Items: items, RepositoryTotals: totals, RawPayload: raw}); err != nil {
				return nil, err
			}
			return map[string]any{"actor_id": actor.Key, "login": login, "items": len(items), "complete": result.Complete, "from": in.From, "to": in.To}, nil
		})
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_user_contributions", "GitHub user contribution synchronization started"), nil
}

// SearchContributions reads contribution observations from the local corpus.
func (r *MCPReader) SearchContributions(ctx context.Context, in mcpcontract.SearchContributionsInput) (mcpcontract.SearchContributionsOutput, error) {
	if len(in.Actors) > 100 || len(in.Repositories) > 100 || len(in.Kinds) > 20 {
		return mcpcontract.SearchContributionsOutput{}, errors.New("actors and repositories are limited to 100 items; kinds is limited to 20")
	}
	if in.Source == "" {
		in.Source = "github_profile"
	}
	if in.Source != "github_profile" {
		return mcpcontract.SearchContributionsOutput{}, errors.New("source must be github_profile; corpus_observation is not yet an indexed contribution source")
	}
	parseBound := func(name, value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be RFC 3339", name)
		}
		return parsed, nil
	}
	from, err := parseBound("from", in.From)
	if err != nil {
		return mcpcontract.SearchContributionsOutput{}, err
	}
	to, err := parseBound("to", in.To)
	if err != nil {
		return mcpcontract.SearchContributionsOutput{}, err
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return mcpcontract.SearchContributionsOutput{}, errors.New("to must be after from")
	}
	repositories := make([]string, len(in.Repositories))
	for i, repository := range in.Repositories {
		if strings.TrimSpace(repository.Owner) == "" || strings.TrimSpace(repository.Repo) == "" {
			return mcpcontract.SearchContributionsOutput{}, fmt.Errorf("repositories[%d] requires owner and repo", i)
		}
		repositories[i] = repository.Owner + "/" + repository.Repo
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchContributionsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.SearchContributionsOutput{}, err
	}
	page, err := c.SearchActorContributions(ctx, corpus.ContributionSearchOptions{ActorRefs: in.Actors, RepositoryRefs: repositories, Kinds: in.Kinds, OrganizationNodeID: in.OrganizationNodeID, From: from, To: to, Sort: in.Sort, Order: in.Order, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return mcpcontract.SearchContributionsOutput{}, err
	}
	out := mcpcontract.SearchContributionsOutput{Items: make([]mcpcontract.ContributionOutput, len(page.Items)), Total: page.Total, NextCursor: page.NextCursor, SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	for i, item := range page.Items {
		out.Items[i] = mcpcontract.ContributionOutput{ActorID: item.ActorKey, Login: item.Login, Kind: item.Kind, Source: "github_profile", OccurredAt: formatTime(item.OccurredAt), RepositoryRef: item.RepositoryRef, TargetNodeID: item.TargetNodeID, TargetURL: item.TargetURL, Restricted: item.Restricted, Count: item.Count}
	}
	coverageActors := in.Actors
	if len(coverageActors) == 0 {
		seen := map[string]bool{}
		for _, item := range page.Items {
			if !seen[item.ActorKey] {
				coverageActors = append(coverageActors, item.ActorKey)
				seen[item.ActorKey] = true
			}
		}
	}
	for _, ref := range coverageActors {
		actor, readErr := c.GetActor(ctx, ref)
		if readErr != nil {
			out.Coverage = append(out.Coverage, mcpcontract.ActorContributionCoverage{ActorID: ref, Facet: mcpcontract.ActorCoverageOutput{Facet: "contributions", Status: "unknown", Reason: "actor_read_failed", PeriodFrom: in.From, PeriodTo: in.To, OrganizationNodeID: in.OrganizationNodeID}})
			continue
		}
		if actor == nil {
			out.Coverage = append(out.Coverage, mcpcontract.ActorContributionCoverage{ActorID: ref, Facet: mcpcontract.ActorCoverageOutput{Facet: "contributions", Status: "unknown", Reason: "actor_not_indexed", PeriodFrom: in.From, PeriodTo: in.To, OrganizationNodeID: in.OrganizationNodeID}})
			continue
		}
		stored, readErr := c.GetActorContributionCoverage(ctx, actor.ID, in.OrganizationNodeID, from, to)
		if readErr != nil {
			return mcpcontract.SearchContributionsOutput{}, readErr
		}
		coverage := mcpcontract.ActorCoverageOutput{Facet: "contributions", Status: "unknown", Reason: "facet_not_synchronized"}
		coverage.PeriodFrom, coverage.PeriodTo = in.From, in.To
		coverage.OrganizationNodeID = in.OrganizationNodeID
		if from.IsZero() || to.IsZero() {
			coverage.Reason = "bounded_period_required"
		} else if stored != nil {
			coverage.Status, coverage.Reason = "complete", ""
			coverage.ObservedAt = formatTime(stored.ObservedAt)
			coverage.SourceUpdatedAt = formatTime(stored.SourceUpdatedAt)
			coverage.AuthorizationScope = stored.AuthorizationScope
		}
		out.Coverage = append(out.Coverage, mcpcontract.ActorContributionCoverage{ActorID: actor.Key, Facet: coverage})
	}
	return out, nil
}

func (r *MCPReader) submitUserFacetJob(ctx context.Context, kind string, in mcpcontract.SyncUserFacetInput, run func(context.Context, *corpus.Corpus, github.Reader, mcpcontract.ActorSelector) (map[string]any, error)) (mcpcontract.JobReference, error) {
	if len(in.Users) < 1 || len(in.Users) > 100 {
		return mcpcontract.JobReference{}, errors.New("users must contain 1 to 100 items")
	}
	if err := validateActorSelectors(in.Users); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if err := normalizeFacetBounds(&in.MaxPages, &in.MaxItems, &in.MaxRequests, len(in.Users)); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, kind, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
		if err != nil {
			return nil, err
		}
		c, err := r.openCorpus(ctx)
		if err != nil {
			return nil, err
		}
		return r.runActorFacetItems(ctx, in.Users, kind, report, func(selector mcpcontract.ActorSelector) (map[string]any, error) { return run(ctx, c, reader, selector) })
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, kind, "GitHub actor facet synchronization started"), nil
}

func (r *MCPReader) runActorFacetItems(ctx context.Context, selectors []mcpcontract.ActorSelector, phase string, report func(string, string) error, run func(mcpcontract.ActorSelector) (map[string]any, error)) (map[string]any, error) {
	items := make([]map[string]any, len(selectors))
	complete := 0
	if err := report(phase, jobProgressCounts(0, len(selectors))); err != nil {
		return nil, err
	}
	for i, selector := range selectors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := run(selector)
		if err != nil {
			itemStatus, reason, message, retry := githubBatchError(err)
			items[i] = map[string]any{"key": actorSelectorKey(selector), "status": itemStatus, "reason": reason, "message": message, "retry_after_ms": retry}
		} else {
			value["key"] = actorSelectorKey(selector)
			value["status"] = "complete"
			items[i] = value
			complete++
		}
		if err := report(phase, jobProgressCounts(i+1, len(selectors))); err != nil {
			return nil, err
		}
	}
	status := "complete"
	if complete != len(selectors) {
		status = "partial"
	}
	return map[string]any{"status": status, "items": items, "completed": complete, "total": len(selectors)}, nil
}

func storedActorForSelector(ctx context.Context, c *corpus.Corpus, selector mcpcontract.ActorSelector) (*corpus.Actor, string, error) {
	login, err := resolveActorSelectorLogin(ctx, c, selector)
	if err != nil {
		return nil, "", err
	}
	actor, err := c.GetActor(ctx, login)
	if err != nil {
		return nil, "", err
	}
	if actor == nil {
		return nil, "", fmt.Errorf("actor %q has no stored identity; call github.sync_users first", login)
	}
	return actor, login, nil
}
func normalizeFacetBounds(maxPages, maxItems, maxRequests *int, userCount int) error {
	if *maxPages == 0 {
		*maxPages = 1
	}
	if *maxItems == 0 {
		*maxItems = 100
	}
	if *maxRequests == 0 {
		*maxRequests = userCount * *maxPages
	}
	if *maxPages < 1 || *maxPages > 10 {
		return errors.New("max_pages must be 1 to 10")
	}
	if *maxItems < 1 || *maxItems > 1000 {
		return errors.New("max_items_per_user must be 1 to 1000")
	}
	if *maxRequests < userCount*(*maxPages) || *maxRequests > 1000 {
		return errors.New("max_requests must cover users times max_pages and cannot exceed 1000")
	}
	return nil
}
func facetMaxPages(value int) int {
	if value <= 0 {
		return 1
	}
	return min(value, 10)
}
func facetMaxItems(value int) int {
	if value <= 0 {
		return 100
	}
	return min(value, 1000)
}
func intPointer(value int) *int { return &value }
