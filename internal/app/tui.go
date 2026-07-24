package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

const maxTUIItems = 100

// Load implements tuicontract.Reader using bounded local corpus reads only.
func (s *Service) Load(ctx context.Context) (tuicontract.Data, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return tuicontract.Data{}, err
	}
	var repos []corpus.Repository
	cursor := ""
	for {
		page, err := c.ListRepositoriesWithOptions(ctx, "", corpus.RepositorySearchOptions{Limit: maxTUIItems, Cursor: cursor, Sort: "updated"})
		if err != nil {
			return tuicontract.Data{}, err
		}
		repos = append(repos, page.Repositories...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	data := tuicontract.Data{
		Repositories: make([]tuicontract.Item, 0, min(len(repos), maxTUIItems)),
		Windows:      make(map[string]tuicontract.Window),
	}
	for _, repo := range repos {
		ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}
		coverage, err := c.ListCoverage(ctx, repo.ID, nil)
		if err != nil {
			return tuicontract.Data{}, err
		}
		itemCoverage := make([]tuicontract.Facet, len(coverage))
		for i, facet := range coverage {
			itemCoverage[i] = tuicontract.Facet{Name: facet.Facet, Present: true, Complete: facet.Complete, AsOf: formatTime(facet.SourceUpdatedAt)}
		}
		if len(data.Repositories) < maxTUIItems {
			data.Repositories = append(data.Repositories, tuicontract.Item{
				Kind: "repository", ID: repo.ExternalID, Ref: ref.String(), Title: ref.String(),
				Subtitle: repo.Language, Detail: repo.Description, Source: "https://github.com/" + ref.String(),
				AsOf: formatTime(repo.SourceUpdatedAt), Coverage: itemCoverage,
			})
		}

		threadTotal, err := c.CountThreadsFiltered(ctx, repo.ID, "", "")
		if err != nil {
			return tuicontract.Data{}, err
		}
		threadWindow := data.Windows["threads"]
		threadWindow.Total += threadTotal
		data.Windows["threads"] = threadWindow
		threads, err := c.ListThreads(ctx, repo.ID, "", maxTUIItems)
		if err != nil {
			return tuicontract.Data{}, err
		}
		for _, thread := range threads {
			data.Threads = append(data.Threads, tuicontract.Item{
				Kind: thread.Kind, ID: fmt.Sprintf("%d", thread.ID), Ref: fmt.Sprintf("%s#%d", ref, thread.Number),
				Title: thread.Title, Subtitle: thread.State + " by " + thread.Author, Detail: thread.Body,
				Source: threadURL(ref, thread.Kind, thread.Number), AsOf: formatTime(thread.SourceUpdatedAt),
			})
		}
	}

	clusters, clusterTotal, err := c.ListRecentClusters(ctx, maxTUIItems)
	if err != nil {
		return tuicontract.Data{}, err
	}
	data.Windows["clusters"] = tuicontract.Window{Total: clusterTotal}
	for _, cluster := range clusters {
		data.Clusters = append(data.Clusters, tuicontract.Item{
			Kind: "cluster", ID: cluster.StableID, Ref: cluster.Repo.String() + ":cluster:" + cluster.StableID,
			Title: cluster.StableID, Subtitle: fmt.Sprintf("%s · %d members", cluster.State, len(cluster.Members)),
			Detail: fmt.Sprintf("Canonical: %s", cluster.Canonical), Source: "local clustering",
			AsOf: formatTime(cluster.UpdatedAt),
		})
	}

	investigations, err := c.ListInvestigations(ctx)
	if err != nil {
		return tuicontract.Data{}, err
	}
	repoByInvestigation := make(map[string]domain.RepoRef, len(investigations))
	for _, inv := range investigations {
		repoByInvestigation[inv.ID] = inv.Repo
		data.Investigations = append(data.Investigations, tuicontract.Item{
			Kind: "investigation", ID: inv.ID, Ref: inv.Repo.String() + ":investigation:" + inv.ID,
			Title: inv.ID, Subtitle: string(inv.Status), Detail: strings.TrimSpace(inv.CommitSHA + " " + inv.Lens),
			Source: "local investigation", AsOf: formatTime(inv.UpdatedAt),
		})
	}
	opportunities, err := c.ListOpportunities(ctx, "")
	if err != nil {
		return tuicontract.Data{}, err
	}
	for _, opportunity := range opportunities {
		repo := repoByInvestigation[opportunity.InvestigationID]
		data.Opportunities = append(data.Opportunities, tuicontract.Item{
			Kind: "opportunity", ID: opportunity.ID, Ref: repo.String() + ":opportunity:" + opportunity.ID,
			Title: opportunity.Title, Subtitle: fmt.Sprintf("%s · confidence %.2f", opportunity.Status, opportunity.Confidence),
			Detail: opportunity.ProblemStatement, Source: "local opportunity", AsOf: formatTime(opportunity.UpdatedAt),
		})
	}

	sortTUIData(&data)
	data.Repositories = capTUIItems(data.Repositories)
	data.Threads = capTUIItems(data.Threads)
	data.Clusters = capTUIItems(data.Clusters)
	data.Investigations = capTUIItems(data.Investigations)
	data.Opportunities = capTUIItems(data.Opportunities)
	data.Windows["repositories"] = tuicontract.Window{Total: len(repos), Truncated: len(repos) > len(data.Repositories)}
	for name, loaded := range map[string]int{
		"threads": len(data.Threads), "clusters": len(data.Clusters),
		"investigations": len(data.Investigations), "opportunities": len(data.Opportunities),
	} {
		window := data.Windows[name]
		switch name {
		case "investigations":
			window.Total = len(investigations)
		case "opportunities":
			window.Total = len(opportunities)
		}
		window.Truncated = window.Total > loaded
		data.Windows[name] = window
	}
	return data, nil
}

func capTUIItems(items []tuicontract.Item) []tuicontract.Item {
	if len(items) > maxTUIItems {
		return items[:maxTUIItems]
	}
	return items
}

func sortTUIData(data *tuicontract.Data) {
	groups := [][]tuicontract.Item{data.Repositories, data.Threads, data.Clusters, data.Investigations, data.Opportunities}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].AsOf != group[j].AsOf {
				return group[i].AsOf > group[j].AsOf
			}
			return group[i].Ref < group[j].Ref
		})
	}
}
