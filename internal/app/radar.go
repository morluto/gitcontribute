package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

const (
	radarCandidatePopulation   = radar.MaxLimit
	radarPullRequestPopulation = 500
)

// ContributionRadar ranks a bounded set of locally stored open issues. It is
// a strict corpus read: it neither resolves a GitHub reader nor writes state.
func (s *Service) ContributionRadar(ctx context.Context, opts cli.RadarOptions) (*radar.Report, error) {
	return s.contributionRadarAt(ctx, opts, s.now())
}

// contributionRadarAt lets one cross-repository ranking use a single scoring
// instant while keeping the public CLI service contract small.
func (s *Service) contributionRadarAt(ctx context.Context, opts cli.RadarOptions, evaluationTime time.Time) (*radar.Report, error) {
	ref := domain.RepoRef{Owner: opts.Repo.Owner, Repo: opts.Repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
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
	relatedByIssue, relationshipScanCapped, err := radarPullRequestRelatedWork(ctx, c, stored, ref, issues, openPullRequests)
	if err != nil {
		return nil, err
	}

	duplicateByIssue, duplicateScanCapped, err := radarDuplicateClusters(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	repositoryCoverage, err := c.ListCoverage(ctx, stored.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("list repository coverage: %w", err)
	}
	storedGuidance, err := readContributionGuidanceDocuments(ctx, c, stored.ID)
	if err != nil {
		return nil, fmt.Errorf("read contribution guidance: %w", err)
	}
	guidance := make([]radar.GuidanceDocument, 0, len(storedGuidance))
	for _, document := range storedGuidance {
		guidance = append(guidance, radar.GuidanceDocument{
			Path: document.File.Path, Content: document.File.Content, URL: document.File.HTMLURL,
		})
	}
	evaluationTime = evaluationTime.UTC()

	snapshots := make([]radar.IssueSnapshot, 0, len(issues))
	for _, issue := range issues {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		coverage, err := c.ListCoverage(ctx, stored.ID, &issue.ID)
		if err != nil {
			return nil, fmt.Errorf("list coverage for issue #%d: %w", issue.Number, err)
		}
		discussion, issueRelated, relatedCapped, err := radarIssueDiscussionAndRelatedWork(ctx, c, stored, issue, ref, evaluationTime)
		if err != nil {
			return nil, fmt.Errorf("read discussion relationships for issue #%d: %w", issue.Number, err)
		}
		related := append(relatedByIssue[issue.Number], issueRelated...)
		if cluster := duplicateByIssue[issue.Number]; cluster != nil {
			related = append(related, radar.RelatedWork{
				Ref: "duplicate_cluster:" + cluster.StableID, Kind: "duplicate_cluster", Title: cluster.CanonicalRef,
				Relation: relatedwork.RelationClusterCandidate, Direction: "local", URL: "local://clusters/" + cluster.StableID,
				Evidence:        []radar.RelatedWorkEvidence{{Kind: "duplicate_cluster", SourceURL: "local://clusters/" + cluster.StableID, SourceAsOf: cluster.SourceAsOf}},
				SourceUpdatedAt: cluster.SourceAsOf,
			})
		}
		related, mergedCapped := normalizeRadarRelatedWork(related, maxRadarRelatedWork)
		linked := radarLinkedPullRequests(related)
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
			Discussion:         discussion,
			LinkedPullRequests: linked,
			RelatedWork:        related,
			RelatedWorkCapped:  relatedCapped || mergedCapped,
			DuplicateCluster:   duplicateByIssue[issue.Number],
		})
	}

	return radar.Rank(radar.RepositorySnapshot{
		Repo:           ref,
		Archived:       stored.Archived,
		SourceUpdated:  stored.SourceUpdatedAt,
		Coverage:       radarCoverage(repositoryCoverage, "repository"),
		GuidanceStatus: radarGuidanceStatus(repositoryCoverage, len(guidance)),
		Guidance:       guidance,
	}, snapshots, radar.Options{
		Limit:                       opts.Limit,
		Now:                         evaluationTime,
		TotalOpenIssues:             totalOpenIssues,
		PopulationCapped:            totalOpenIssues > len(issues),
		LinkedPullRequestScanCapped: totalOpenPullRequests > len(openPullRequests) || relationshipScanCapped,
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

func radarGuidanceStatus(coverage []corpus.Coverage, documentCount int) string {
	for _, item := range coverage {
		if item.Facet != FacetContributionGuidance || !item.Complete {
			continue
		}
		if documentCount == 0 {
			return "missing"
		}
		return "available"
	}
	return "unknown"
}

func radarLinkedPullRequests(values []radar.RelatedWork) []radar.LinkedPullRequest {
	out := []radar.LinkedPullRequest{}
	for _, value := range values {
		if value.Kind != string(domain.PullRequestKind) || value.Direction != "inbound" || !strings.EqualFold(value.State, "open") {
			continue
		}
		out = append(out, radar.LinkedPullRequest{
			Number: value.Number, Title: value.Title, URL: value.URL,
			Closing: value.Relation == "claims_to_close", SourceUpdatedAt: value.SourceUpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func radarDuplicateClusters(ctx context.Context, c *corpus.Corpus, ref domain.RepoRef) (map[int]*radar.DuplicateCluster, bool, error) {
	projection, err := c.ListClusterProjection(ctx, ref, clustering.ClusterOpen, 1000)
	if err != nil {
		return nil, false, fmt.Errorf("list duplicate clusters: %w", err)
	}
	out := make(map[int]*radar.DuplicateCluster)
	clusters := projection.Clusters
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
