package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/tui"
)

const maxTUIItems = 100

// Load implements tui.Reader using bounded local corpus reads only.
func (s *Service) Load(ctx context.Context) (tui.Data, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return tui.Data{}, err
	}
	repos, err := c.ListRepositories(ctx, "", maxTUIItems)
	if err != nil {
		return tui.Data{}, err
	}
	data := tui.Data{Repositories: make([]tui.Item, 0, len(repos))}
	for _, repo := range repos {
		ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}
		coverage, err := c.ListCoverage(ctx, repo.ID, nil)
		if err != nil {
			return tui.Data{}, err
		}
		itemCoverage := make([]tui.Facet, len(coverage))
		for i, facet := range coverage {
			itemCoverage[i] = tui.Facet{Name: facet.Facet, Present: true, Complete: facet.Complete, AsOf: formatTime(facet.SourceUpdatedAt)}
		}
		data.Repositories = append(data.Repositories, tui.Item{
			Kind: "repository", ID: repo.ExternalID, Ref: ref.String(), Title: ref.String(),
			Subtitle: repo.Language, Detail: repo.Description, Source: "https://github.com/" + ref.String(),
			AsOf: formatTime(repo.SourceUpdatedAt), Coverage: itemCoverage,
		})

		remaining := maxTUIItems - len(data.Threads)
		if remaining > 0 {
			threads, err := c.ListThreads(ctx, repo.ID, "", remaining)
			if err != nil {
				return tui.Data{}, err
			}
			for _, thread := range threads {
				data.Threads = append(data.Threads, tui.Item{
					Kind: thread.Kind, ID: fmt.Sprintf("%d", thread.ID), Ref: fmt.Sprintf("%s#%d", ref, thread.Number),
					Title: thread.Title, Subtitle: thread.State + " by " + thread.Author, Detail: thread.Body,
					Source: threadURL(ref, thread.Kind, thread.Number), AsOf: formatTime(thread.SourceUpdatedAt),
				})
			}
		}

		remaining = maxTUIItems - len(data.Clusters)
		if remaining > 0 {
			projection, err := c.ListClusterProjection(ctx, ref, clustering.ClusterState(""), remaining)
			if err != nil {
				return tui.Data{}, err
			}
			for _, cluster := range projection.Clusters {
				data.Clusters = append(data.Clusters, tui.Item{
					Kind: "cluster", ID: cluster.StableID, Ref: ref.String() + ":cluster:" + cluster.StableID,
					Title: cluster.StableID, Subtitle: fmt.Sprintf("%s · %d members", cluster.State, len(cluster.Members)),
					Detail: fmt.Sprintf("Canonical: %s", cluster.Canonical), Source: "local clustering",
					AsOf: formatTime(cluster.UpdatedAt),
				})
			}
		}
	}

	investigations, err := c.ListInvestigations(ctx)
	if err != nil {
		return tui.Data{}, err
	}
	if len(investigations) > maxTUIItems {
		investigations = investigations[:maxTUIItems]
	}
	repoByInvestigation := make(map[string]domain.RepoRef, len(investigations))
	for _, inv := range investigations {
		repoByInvestigation[inv.ID] = inv.Repo
		data.Investigations = append(data.Investigations, tui.Item{
			Kind: "investigation", ID: inv.ID, Ref: inv.Repo.String() + ":investigation:" + inv.ID,
			Title: inv.ID, Subtitle: string(inv.Status), Detail: strings.TrimSpace(inv.CommitSHA + " " + inv.Lens),
			Source: "local investigation", AsOf: formatTime(inv.UpdatedAt),
		})
	}
	opportunities, err := c.ListOpportunities(ctx, "")
	if err != nil {
		return tui.Data{}, err
	}
	if len(opportunities) > maxTUIItems {
		opportunities = opportunities[:maxTUIItems]
	}
	for _, opportunity := range opportunities {
		repo := repoByInvestigation[opportunity.InvestigationID]
		data.Opportunities = append(data.Opportunities, tui.Item{
			Kind: "opportunity", ID: opportunity.ID, Ref: repo.String() + ":opportunity:" + opportunity.ID,
			Title: opportunity.Title, Subtitle: fmt.Sprintf("%s · confidence %.2f", opportunity.Status, opportunity.Confidence),
			Detail: opportunity.ProblemStatement, Source: "local opportunity", AsOf: formatTime(opportunity.UpdatedAt),
		})
	}

	sortTUIData(&data)
	return data, nil
}

func sortTUIData(data *tui.Data) {
	groups := [][]tui.Item{data.Repositories, data.Threads, data.Clusters, data.Investigations, data.Opportunities}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].AsOf != group[j].AsOf {
				return group[i].AsOf > group[j].AsOf
			}
			return group[i].Ref < group[j].Ref
		})
	}
}
