package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/tracking"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

const (
	maxTUIItems              = 100
	maxTUIRadarRepositories  = 10
	maxTUIRadarPerRepository = 20
)

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
		if len(data.SyncStatuses) < maxTUIItems {
			syncStatus, err := buildTUISyncStatus(ctx, c, repo, ref, coverage)
			if err != nil {
				return tuicontract.Data{}, err
			}
			data.SyncStatuses = append(data.SyncStatuses, syncStatus)
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
				Status: thread.State, Source: threadURL(ref, thread.Kind, thread.Number), AsOf: formatTime(thread.SourceUpdatedAt),
			})
		}
	}

	evaluationTime := s.now().UTC()
	radarRepos := repos
	if len(radarRepos) > maxTUIRadarRepositories {
		radarRepos = radarRepos[:maxTUIRadarRepositories]
	}
	for _, repo := range radarRepos {
		report, err := s.contributionRadarAt(ctx, contracts.RadarOptions{
			Repo:  contracts.RepoRef{Owner: repo.Owner, Repo: repo.Name},
			Limit: maxTUIRadarPerRepository,
		}, evaluationTime)
		if err != nil {
			return tuicontract.Data{}, fmt.Errorf("build TUI contribution radar for %s/%s: %w", repo.Owner, repo.Name, err)
		}
		window := data.Windows["candidates"]
		window.Total += report.TotalOpenIssues
		window.Truncated = window.Truncated || report.PopulationCapped || len(report.Candidates) < report.TotalOpenIssues
		data.Windows["candidates"] = window
		for _, candidate := range report.Candidates {
			data.Candidates = append(data.Candidates, radarCandidateItem(candidate))
		}
	}
	if len(repos) > len(radarRepos) {
		window := data.Windows["candidates"]
		window.Truncated = true
		data.Windows["candidates"] = window
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
	opportunityRepo := make(map[string]domain.RepoRef)
	opportunities, err := c.ListOpportunities(ctx, "")
	if err != nil {
		return tuicontract.Data{}, err
	}
	for _, inv := range investigations {
		repoByInvestigation[inv.ID] = inv.Repo
		data.Investigations = append(data.Investigations, tuicontract.Item{
			Kind: "investigation", ID: inv.ID, Ref: inv.Repo.String() + ":investigation:" + inv.ID,
			Title: investigationTitle(inv), Subtitle: string(inv.Status), Detail: strings.TrimSpace(inv.CommitSHA + " " + inv.Lens),
			Status: string(inv.Status), Stage: "active",
			Source: "local investigation", AsOf: formatTime(inv.UpdatedAt),
		})
		hypotheses, err := c.ListHypotheses(ctx, inv.ID)
		if err != nil {
			return tuicontract.Data{}, err
		}
		for _, hypothesis := range hypotheses {
			if hypothesis.Status != "proposed" && hypothesis.Status != "deferred" {
				continue
			}
			data.Hypotheses = append(data.Hypotheses, tuicontract.Item{
				Kind: "hypothesis", ID: hypothesis.ID, Ref: inv.Repo.String() + ":hypothesis:" + hypothesis.ID,
				Title: hypothesis.Title, Subtitle: string(hypothesis.Category) + " · " + string(hypothesis.Status),
				Detail: hypothesis.Description, Status: string(hypothesis.Status), Stage: "research",
				Source: "local hypothesis", AsOf: formatTime(hypothesis.UpdatedAt),
			})
		}
	}
	for _, opportunity := range opportunities {
		opportunityRepo[opportunity.ID] = repoByInvestigation[opportunity.InvestigationID]
	}
	displayOpportunities := opportunities
	if len(displayOpportunities) > maxTUIItems {
		displayOpportunities = displayOpportunities[:maxTUIItems]
	}
	for _, opportunity := range displayOpportunities {
		repo := repoByInvestigation[opportunity.InvestigationID]
		item := tuicontract.Item{
			Kind: "opportunity", ID: opportunity.ID, Ref: repo.String() + ":opportunity:" + opportunity.ID,
			Title: opportunity.Title, Subtitle: fmt.Sprintf("%s · confidence %.2f", opportunity.Status, opportunity.Confidence),
			Detail: opportunity.ProblemStatement, Status: string(opportunity.Status), Confidence: fmt.Sprintf("%.2f", opportunity.Confidence),
			Stage: "validate", Source: "local opportunity", AsOf: formatTime(opportunity.UpdatedAt),
		}
		readiness, readinessErr := s.OpportunityReadiness(ctx, opportunity.ID)
		if readinessErr != nil {
			if err := ctx.Err(); err != nil {
				return tuicontract.Data{}, err
			}
			item.Assessment = &tuicontract.Assessment{Unknowns: []tuicontract.Fact{{
				Code: "readiness_unavailable", Summary: "Readiness could not be evaluated from the local corpus.",
			}}}
		} else {
			item.Stage = "validate"
			if readiness.Status == "pass" {
				item.Stage = "ready"
			}
			item.Status = readiness.Status
			item.Assessment = readinessAssessment(readiness.Checks)
		}
		data.Opportunities = append(data.Opportunities, item)
	}

	contributions, err := c.ListContributions(ctx, tracking.ContributionFilter{Limit: maxTUIItems})
	if err != nil {
		return tuicontract.Data{}, err
	}
	for _, contribution := range contributions {
		repo := opportunityRepo[contribution.OpportunityID]
		status := "prepared"
		if contribution.SubmittedAt != nil {
			status = "submitted"
		}
		data.Contributions = append(data.Contributions, tuicontract.Item{
			Kind: "contribution", ID: contribution.ID, Ref: contribution.Reference,
			Title: contribution.Title, Subtitle: contribution.Kind + " · " + status,
			Detail: contribution.Body, Status: status, Stage: "submitted",
			Source: contribution.ReferenceURL, AsOf: formatTime(contribution.UpdatedAt),
		})
		if data.Contributions[len(data.Contributions)-1].Ref == "" {
			data.Contributions[len(data.Contributions)-1].Ref = repo.String() + ":contribution:" + contribution.ID
		}
	}

	sortTUIData(&data)
	candidateCount := len(data.Candidates)
	hypothesisCount := len(data.Hypotheses)
	data.Candidates = capTUIItems(data.Candidates)
	data.Hypotheses = capTUIItems(data.Hypotheses)
	data.Contributions = capTUIItems(data.Contributions)
	data.Repositories = capTUIItems(data.Repositories)
	data.SyncStatuses = capTUIItems(data.SyncStatuses)
	data.Threads = capTUIItems(data.Threads)
	data.Clusters = capTUIItems(data.Clusters)
	data.Investigations = capTUIItems(data.Investigations)
	data.Opportunities = capTUIItems(data.Opportunities)
	data.Windows["repositories"] = tuicontract.Window{Total: len(repos), Truncated: len(repos) > len(data.Repositories)}
	data.Windows["sync_statuses"] = tuicontract.Window{Total: len(repos), Truncated: len(repos) > len(data.SyncStatuses)}
	candidateWindow := data.Windows["candidates"]
	candidateWindow.Truncated = candidateWindow.Truncated || candidateCount > len(data.Candidates)
	data.Windows["candidates"] = candidateWindow
	data.Windows["hypotheses"] = tuicontract.Window{Total: hypothesisCount, Truncated: hypothesisCount > len(data.Hypotheses)}
	data.Windows["contributions"] = tuicontract.Window{Total: len(contributions), Truncated: len(contributions) > len(data.Contributions)}
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
	if err := s.attachTUIActions(ctx, &data); err != nil {
		return tuicontract.Data{}, err
	}
	return data, nil
}

func (s *Service) attachTUIActions(ctx context.Context, data *tuicontract.Data) error {
	if data == nil {
		return nil
	}
	for _, items := range [][]tuicontract.Item{
		data.Candidates,
		data.Hypotheses,
		data.SyncStatuses,
		data.Contributions,
		data.Repositories,
		data.Threads,
		data.Clusters,
		data.Investigations,
		data.Opportunities,
	} {
		for i := range items {
			actions, err := s.Actions(ctx, items[i])
			if err != nil {
				return fmt.Errorf("load TUI actions for %s %s: %w", items[i].Kind, items[i].Ref, err)
			}
			items[i].Actions = actions
		}
	}
	return nil
}

func capTUIItems(items []tuicontract.Item) []tuicontract.Item {
	if len(items) > maxTUIItems {
		return items[:maxTUIItems]
	}
	return items
}

func sortTUIData(data *tuicontract.Data) {
	groups := [][]tuicontract.Item{
		data.Candidates, data.Hypotheses, data.Contributions, data.Repositories,
		data.SyncStatuses, data.Threads, data.Clusters, data.Investigations, data.Opportunities,
	}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Score != group[j].Score {
				return group[i].Score > group[j].Score
			}
			if group[i].AsOf != group[j].AsOf {
				return group[i].AsOf > group[j].AsOf
			}
			return group[i].Ref < group[j].Ref
		})
	}
}

func radarCandidateItem(candidate radar.Candidate) tuicontract.Item {
	assessment := &tuicontract.Assessment{
		Positive: radarSignalFacts(candidate.PositiveSignals),
		Risks:    radarSignalFacts(candidate.Risks),
		Blockers: radarSignalFacts(candidate.Blockers),
	}
	for _, unknown := range candidate.Unknowns {
		assessment.Unknowns = append(assessment.Unknowns, tuicontract.Fact{
			Code: unknown.Code, Summary: unknown.Summary,
		})
	}
	for _, related := range candidate.RelatedWork {
		summary := related.Ref
		if related.Title != "" {
			summary = related.Title
		}
		if related.Relation != "" {
			summary += " · " + strings.ReplaceAll(related.Relation, "_", " ")
		}
		assessment.Related = append(assessment.Related, tuicontract.Fact{
			Code: related.Kind, Summary: summary, Source: related.URL,
		})
	}
	return tuicontract.Item{
		Kind: "candidate", ID: candidate.Ref, Ref: candidate.Ref, Title: candidate.Title,
		Subtitle: strings.ReplaceAll(string(candidate.Eligibility), "_", " ") + " · score " + fmt.Sprint(candidate.Score),
		Status:   string(candidate.Eligibility), Stage: "discover", Score: candidate.Score,
		Confidence: candidate.Confidence, Source: candidate.URL, AsOf: formatTime(candidate.SourceUpdatedAt),
		Assessment: assessment,
	}
}

func radarSignalFacts(signals []radar.Signal) []tuicontract.Fact {
	facts := make([]tuicontract.Fact, 0, len(signals))
	for _, signal := range signals {
		facts = append(facts, tuicontract.Fact{
			Code: signal.Code, Summary: signal.Summary, Source: signal.SourceURL,
		})
	}
	return facts
}

func readinessAssessment(checks []contracts.ReadinessCheck) *tuicontract.Assessment {
	assessment := &tuicontract.Assessment{}
	for _, check := range checks {
		fact := tuicontract.Fact{Code: check.RuleID, Summary: check.Summary}
		switch check.Status {
		case "pass":
			assessment.Positive = append(assessment.Positive, fact)
		case "warn":
			assessment.Risks = append(assessment.Risks, fact)
		case "block":
			assessment.Blockers = append(assessment.Blockers, fact)
		default:
			assessment.Unknowns = append(assessment.Unknowns, fact)
		}
	}
	return assessment
}

func investigationTitle(inv *investigation.Investigation) string {
	if inv.ThreadBaseline != nil {
		return inv.ThreadBaseline.Ref()
	}
	return inv.ID
}
