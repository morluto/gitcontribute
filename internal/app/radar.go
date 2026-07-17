package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/radar"
)

const (
	radarCandidatePopulation   = 500
	radarPullRequestPopulation = 500
)

var closingReferencePattern = regexp.MustCompile(`(?i)\b(?:close(?:s|d)?|fix(?:es|ed)?|resolve(?:s|d)?)\s*:?\s*((?:(?:https?://)?(?:www\.)?github\.com/[a-z0-9](?:[a-z0-9-]*[a-z0-9])?/[a-z0-9_.-]+/(?:issues|pull)/\d+)|(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?/[a-z0-9_.-]+#\s*\d+)|(?:#\s*\d+))`)

// ContributionRadar ranks a bounded set of locally stored open issues. It is
// a strict corpus read: it neither resolves a GitHub reader nor writes state.
func (s *Service) ContributionRadar(ctx context.Context, opts cli.RadarOptions) (*radar.Report, error) {
	ref := domain.RepoRef{Owner: opts.Repo.Owner, Repo: opts.Repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	totalOpenIssues, err := c.CountThreadsFiltered(ctx, stored.ID, corpus.ThreadKindIssue, "open")
	if err != nil {
		return nil, fmt.Errorf("count radar issues: %w", err)
	}
	issues, err := c.ListThreadsFiltered(ctx, stored.ID, corpus.ThreadKindIssue, "open", radarCandidatePopulation)
	if err != nil {
		return nil, fmt.Errorf("list radar issues: %w", err)
	}

	totalOpenPullRequests, err := c.CountThreadsFiltered(ctx, stored.ID, corpus.ThreadKindPullRequest, "open")
	if err != nil {
		return nil, fmt.Errorf("count open pull requests: %w", err)
	}
	openPullRequests, err := c.ListThreadsFiltered(ctx, stored.ID, corpus.ThreadKindPullRequest, "open", radarPullRequestPopulation)
	if err != nil {
		return nil, fmt.Errorf("list open pull requests: %w", err)
	}
	linkedByIssue := radarPullRequestLinks(ref, openPullRequests)

	duplicateByIssue, duplicateScanCapped, err := radarDuplicateClusters(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	repositoryCoverage, err := c.ListCoverage(ctx, stored.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("list repository coverage: %w", err)
	}

	snapshots := make([]radar.IssueSnapshot, 0, len(issues))
	for _, issue := range issues {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		coverage, err := c.ListCoverage(ctx, stored.ID, &issue.ID)
		if err != nil {
			return nil, fmt.Errorf("list coverage for issue #%d: %w", issue.Number, err)
		}
		maintainerResponse, replyURL, err := radarMaintainerResponse(ctx, c, stored.ID, issue.ID)
		if err != nil {
			return nil, fmt.Errorf("read comments for issue #%d: %w", issue.Number, err)
		}
		snapshots = append(snapshots, radar.IssueSnapshot{
			Number:             issue.Number,
			State:              issue.State,
			Title:              issue.Title,
			Body:               issue.Body,
			Labels:             issue.Labels,
			Assignees:          issue.Assignees,
			Locked:             issue.Locked,
			SourceUpdated:      issue.SourceUpdatedAt,
			URL:                fmt.Sprintf("https://github.com/%s/issues/%d", ref, issue.Number),
			Coverage:           radarCoverage(coverage, "thread"),
			MaintainerResponse: maintainerResponse,
			MaintainerReplyURL: replyURL,
			LinkedPullRequests: linkedByIssue[issue.Number],
			DuplicateCluster:   duplicateByIssue[issue.Number],
		})
	}

	return radar.Rank(radar.RepositorySnapshot{
		Repo:           ref,
		Archived:       stored.Archived,
		SourceUpdated:  stored.SourceUpdatedAt,
		Coverage:       radarCoverage(repositoryCoverage, "repository"),
		GuidanceStatus: "unknown",
	}, snapshots, radar.Options{
		Limit:                       opts.Limit,
		Now:                         s.now(),
		TotalOpenIssues:             totalOpenIssues,
		PopulationCapped:            totalOpenIssues > len(issues),
		LinkedPullRequestScanCapped: totalOpenPullRequests > len(openPullRequests),
		DuplicateClusterScanCapped:  duplicateScanCapped,
	})
}

func radarCoverage(items []corpus.Coverage, scope string) []radar.Coverage {
	out := make([]radar.Coverage, 0, len(items))
	for _, item := range items {
		out = append(out, radar.Coverage{
			Facet: item.Facet, Scope: scope, Present: true, Complete: item.Complete, AsOf: item.SourceUpdatedAt,
		})
	}
	return out
}

func radarMaintainerResponse(ctx context.Context, c *corpus.Corpus, repoID, threadID int64) (bool, string, error) {
	observations, err := c.ListFacetObservations(ctx, repoID, &threadID, FacetIssueComments)
	if err != nil {
		return false, "", err
	}
	for _, observation := range observations {
		var comments []github.IssueComment
		if err := json.Unmarshal([]byte(observation.Payload), &comments); err != nil {
			return false, "", fmt.Errorf("decode observation %d: %w", observation.ID, err)
		}
		for _, comment := range comments {
			switch strings.ToUpper(comment.AuthorAssociation) {
			case "OWNER", "MEMBER", "COLLABORATOR":
				return true, comment.HTMLURL, nil
			}
		}
	}
	return false, "", nil
}

func radarPullRequestLinks(ref domain.RepoRef, pullRequests []corpus.Thread) map[int][]radar.LinkedPullRequest {
	out := make(map[int][]radar.LinkedPullRequest)
	for _, pullRequest := range pullRequests {
		text := pullRequest.Title + "\n" + pullRequest.Body
		closing := radarClosingIssueNumbers(text, ref)
		seen := make(map[int]struct{})
		for _, linked := range clustering.ExtractRefs(text, ref) {
			if !strings.EqualFold(linked.Owner, ref.Owner) || !strings.EqualFold(linked.Repo, ref.Repo) || linked.Kind == corpus.ThreadKindPullRequest {
				continue
			}
			if _, ok := seen[linked.Number]; ok {
				continue
			}
			seen[linked.Number] = struct{}{}
			_, closes := closing[linked.Number]
			out[linked.Number] = append(out[linked.Number], radar.LinkedPullRequest{
				Number:          pullRequest.Number,
				Title:           pullRequest.Title,
				URL:             fmt.Sprintf("https://github.com/%s/pull/%d", ref, pullRequest.Number),
				Closing:         closes,
				SourceUpdatedAt: pullRequest.SourceUpdatedAt,
			})
		}
	}
	for number := range out {
		sort.Slice(out[number], func(i, j int) bool { return out[number][i].Number < out[number][j].Number })
	}
	return out
}

func radarClosingIssueNumbers(text string, ref domain.RepoRef) map[int]struct{} {
	out := make(map[int]struct{})
	for _, match := range closingReferencePattern.FindAllStringSubmatch(text, -1) {
		for _, linked := range clustering.ExtractRefs(match[1], ref) {
			if strings.EqualFold(linked.Owner, ref.Owner) && strings.EqualFold(linked.Repo, ref.Repo) && linked.Kind != corpus.ThreadKindPullRequest {
				out[linked.Number] = struct{}{}
			}
		}
	}
	return out
}

func radarDuplicateClusters(ctx context.Context, c *corpus.Corpus, ref domain.RepoRef) (map[int]*radar.DuplicateCluster, bool, error) {
	clusters, err := c.Clustering().ListClusters(ctx, ref, clustering.ClusterOpen, 1000)
	if err != nil {
		return nil, false, fmt.Errorf("list duplicate clusters: %w", err)
	}
	out := make(map[int]*radar.DuplicateCluster)
	for _, cluster := range clusters {
		included := 0
		for _, member := range cluster.Members {
			if member.Included {
				included++
			}
		}
		if included < 2 {
			continue
		}
		for _, member := range cluster.Members {
			if !member.Included || member.Ref.Kind != corpus.ThreadKindIssue || !strings.EqualFold(member.Ref.Owner, ref.Owner) || !strings.EqualFold(member.Ref.Repo, ref.Repo) {
				continue
			}
			fact := &radar.DuplicateCluster{
				StableID: cluster.StableID, CanonicalRef: radarClusterMemberRef(cluster.Canonical), CandidateCount: max(0, included-1), SourceAsOf: cluster.WindowEnd,
			}
			current := out[member.Ref.Number]
			if current == nil || fact.StableID < current.StableID {
				out[member.Ref.Number] = fact
			}
		}
	}
	return out, len(clusters) == 1000, nil
}

func radarClusterMemberRef(ref clustering.MemberRef) string {
	return fmt.Sprintf("%s:%s/%s#%d", ref.Kind, ref.Owner, ref.Repo, ref.Number)
}
