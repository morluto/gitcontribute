package dossier

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// DefaultRecentLimit is the number of threads kept in each dossier section.
const DefaultRecentLimit = 10

// Builder assembles deterministic, source-backed repository dossiers.
type Builder struct {
	reader      Reader
	recentLimit int
}

// NewBuilder returns a Builder backed by reader. recentLimit defaults to
// DefaultRecentLimit when zero or negative.
func NewBuilder(reader Reader, recentLimit int) *Builder {
	if recentLimit <= 0 {
		recentLimit = DefaultRecentLimit
	}
	return &Builder{reader: reader, recentLimit: recentLimit}
}

// Build constructs a Dossier for ref. It validates the repo reference, reads
// repository metadata, contribution guidance, coverage, and threads, then
// deterministically selects and orders recent items. No LLM summarization is
// performed.
func (b *Builder) Build(ctx context.Context, ref domain.RepoRef) (*domain.Dossier, error) {
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("invalid repo reference: %w", err)
	}

	repo, repoRefs, err := b.reader.ReadRepository(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("read repository: %w", err)
	}

	coverage, err := b.reader.ReadCoverage(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("read coverage: %w", err)
	}

	guidance, guideRefs, err := b.reader.ReadContributionGuidance(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("read contribution guidance: %w", err)
	}

	closed, closedRefs, err := b.readPullRequests(ctx, ref, domain.ClosedState, nil)
	if err != nil {
		return nil, err
	}
	merged, closedUnmerged, unknownMerge := partitionClosedPullRequests(closed)
	open, openRefs, err := b.readPullRequests(ctx, ref, domain.OpenState, nil)
	if err != nil {
		return nil, err
	}
	issues, issueRefs, err := b.readIssues(ctx, ref)
	if err != nil {
		return nil, err
	}

	refs := collectRefs(repoRefs, guideRefs, closedRefs, openRefs, issueRefs)

	d := &domain.Dossier{
		Repo:                             ref,
		CommitSHA:                        repo.CommitSHA,
		AsOf:                             latestTime(refs),
		SourceRefs:                       refs,
		Coverage:                         coverage,
		Repository:                       repo,
		ContributionGuidance:             guidance,
		OpenIssueCount:                   repo.OpenIssueCount,
		ClosedIssueCount:                 repo.ClosedIssueCount,
		OpenPullRequestCount:             repo.OpenPullRequestCount,
		MergedPullRequestCount:           repo.MergedPullRequestCount,
		ClosedUnmergedPullRequestCount:   repo.ClosedUnmergedPullRequestCount,
		ClosedPullRequestUnknownCount:    repo.ClosedPullRequestUnknownCount,
		RecentMergedPullRequests:         toDossierThreads(merged, b.recentLimit),
		RecentOpenPullRequests:           toDossierThreads(open, b.recentLimit),
		RecentClosedUnmergedPullRequests: toDossierThreads(closedUnmerged, b.recentLimit),
		RecentClosedUnknownPullRequests:  toDossierThreads(unknownMerge, b.recentLimit),
		RecentIssues:                     toDossierThreads(issues, b.recentLimit),
	}
	return d, nil
}

func (b *Builder) readPullRequests(ctx context.Context, ref domain.RepoRef, state domain.ThreadState, merged *bool) ([]domain.Thread, []domain.SourceRef, error) {
	threads, refs, err := b.reader.ReadThreads(ctx, ref, ThreadQuery{
		Kind:   domain.PullRequestKind,
		State:  state,
		Merged: merged,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read PRs (state=%s merged=%s): %w", state, boolStr(merged), err)
	}
	return threads, refs, nil
}

func boolStr(p *bool) string {
	if p == nil {
		return "any"
	}
	if *p {
		return "true"
	}
	return "false"
}

func (b *Builder) readIssues(ctx context.Context, ref domain.RepoRef) ([]domain.Thread, []domain.SourceRef, error) {
	threads, refs, err := b.reader.ReadThreads(ctx, ref, ThreadQuery{
		Kind: domain.IssueKind,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read issues: %w", err)
	}
	return threads, refs, nil
}

func partitionClosedPullRequests(threads []domain.Thread) (merged, unmerged, unknown []domain.Thread) {
	for _, thread := range threads {
		if thread.PullRequest == nil || !thread.PullRequest.MergedKnown {
			unknown = append(unknown, thread)
		} else if thread.PullRequest.Merged {
			merged = append(merged, thread)
		} else {
			unmerged = append(unmerged, thread)
		}
	}
	return merged, unmerged, unknown
}

// toDossierThreads sorts threads deterministically and returns the first limit.
// It copies the input so the Reader's returned slices are not mutated.
func toDossierThreads(threads []domain.Thread, limit int) []domain.DossierThread {
	threads = append([]domain.Thread(nil), threads...)
	sortThreads(threads)
	if limit > 0 && len(threads) > limit {
		threads = threads[:limit]
	}
	out := make([]domain.DossierThread, len(threads))
	for i, t := range threads {
		dt := domain.DossierThread{
			Number:    t.Number,
			Title:     t.Title,
			Author:    t.Author,
			State:     t.State,
			Draft:     t.Draft,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
			ClosedAt:  t.ClosedAt,
			Labels:    append([]string(nil), t.Labels...),
		}
		if t.PullRequest != nil {
			dt.MergedAt = t.PullRequest.MergedAt
		}
		out[i] = dt
	}
	return out
}

// sortOrders defines deterministic thread ordering: newest update first, then
// newest creation, then higher number, then title ascending.
func sortThreads(threads []domain.Thread) {
	sort.SliceStable(threads, func(i, j int) bool {
		a, b := threads[i], threads[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.Number != b.Number {
			return a.Number > b.Number
		}
		return a.Title < b.Title
	})
}

func collectRefs(groups ...[]domain.SourceRef) []domain.SourceRef {
	seen := make(map[string]struct{})
	var out []domain.SourceRef
	for _, group := range groups {
		for _, r := range group {
			key := r.Source + "|" + r.URL + "|" + r.CommitSHA + "|" + r.ObservedAt.Format(time.RFC3339Nano)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ObservedAt.Equal(out[j].ObservedAt) {
			return out[i].ObservedAt.Before(out[j].ObservedAt)
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func latestTime(refs []domain.SourceRef) time.Time {
	var latest time.Time
	for _, r := range refs {
		if r.AsOf.After(latest) {
			latest = r.AsOf
		}
		if r.ObservedAt.After(latest) {
			latest = r.ObservedAt
		}
	}
	return latest
}
