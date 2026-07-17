package health

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

const (
	defaultStaleThreshold = 14 * 24 * time.Hour
	threadListLimit       = 10000

	facetIssueComments    = "issue_comments"
	facetPRReviews        = "pr_reviews"
	facetPRReviewComments = "pr_review_comments"
)

// Compute builds a deterministic health report from the stored corpus facts for
// the repository identified by repoID. It never performs network access.
func Compute(ctx context.Context, c *corpus.Corpus, repoID int64, opts Options) (*Report, error) {
	repo, err := c.GetRepositoryByID(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return nil, fmt.Errorf("repository not found")
	}

	threads, err := c.ListThreads(ctx, repoID, "", threadListLimit)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	truncated := len(threads) == threadListLimit

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	end := opts.End
	if end.IsZero() {
		end = now
	}

	windowStart, windowLabel := resolveWindowStart(opts.Start, threads, repo)
	window := Window{Start: windowStart, End: end, Label: windowLabel}
	staleThreshold := opts.StaleThreshold
	if staleThreshold == 0 {
		staleThreshold = defaultStaleThreshold
	}

	report := &Report{
		Repo:        domain.RepoRef{Owner: repo.Owner, Repo: repo.Name},
		GeneratedAt: now,
		Window:      window,
		Repository: RepositoryMetrics{
			Present:       true,
			Stars:         repo.Stars,
			Watchers:      repo.Watchers,
			Forks:         repo.Forks,
			OpenIssues:    repo.OpenIssues,
			Archived:      repo.Archived,
			Fork:          repo.Fork,
			DefaultBranch: repo.DefaultBranch,
			License:       repo.License,
			Coverage:      "repository",
		},
		Coverage: CoverageSummary{
			ThreadsLimit:         threadListLimit,
			ThreadsTruncated:     truncated,
			ThreadsSampleSize:    len(threads),
			RepositoryProjection: true,
		},
	}

	issueMetrics, prMetrics := countThreads(threads, window, truncated)
	report.Issues = issueMetrics
	report.PullRequests = prMetrics

	external := computeExternalMetrics(threads, opts.Start, end)
	report.External = external
	report.External.Window = window

	congestion := computeCongestion(threads, now, window)
	report.Congestion = congestion

	stale, err := computeStaleSignals(ctx, c, threads, now, staleThreshold, window)
	if err != nil {
		return nil, fmt.Errorf("stale signals: %w", err)
	}
	report.Stale = stale

	response, err := computeResponseTimes(ctx, c, threads, opts.Start, end, window)
	if err != nil {
		return nil, fmt.Errorf("response times: %w", err)
	}
	report.Response = response

	return report, nil
}

func resolveWindowStart(startOpt time.Time, threads []corpus.Thread, repo *corpus.Repository) (time.Time, string) {
	if !startOpt.IsZero() {
		return startOpt, fmt.Sprintf("since %s", startOpt.Format(time.RFC3339))
	}
	earliest := repo.SourceCreatedAt
	for _, t := range threads {
		if !t.SourceCreatedAt.IsZero() && (earliest.IsZero() || t.SourceCreatedAt.Before(earliest)) {
			earliest = t.SourceCreatedAt
		}
	}
	if earliest.IsZero() {
		return earliest, "all observed history"
	}
	return earliest, fmt.Sprintf("all observed history since %s", earliest.Format(time.RFC3339))
}

func countThreads(threads []corpus.Thread, window Window, truncated bool) (IssueMetrics, PullRequestMetrics) {
	issueMetrics := IssueMetrics{Window: window}
	prMetrics := PullRequestMetrics{Window: window}
	missingCreated := false
	for _, t := range threads {
		if t.SourceCreatedAt.IsZero() {
			missingCreated = true
			continue
		}
		if !withinWindow(t.SourceCreatedAt, window.Start, window.End) {
			continue
		}
		switch t.Kind {
		case corpus.ThreadKindIssue:
			issueMetrics.SampleSize++
			if t.State == "open" {
				issueMetrics.Open++
			} else {
				issueMetrics.Closed++
			}
		case corpus.ThreadKindPullRequest:
			prMetrics.SampleSize++
			if t.State == "open" {
				prMetrics.Open++
			} else if t.Merged {
				prMetrics.Merged++
			} else {
				prMetrics.ClosedUnmerged++
			}
		}
	}
	coverage := "complete"
	if truncated {
		coverage = "partial (thread list may be truncated)"
	} else if missingCreated {
		coverage = "partial (some threads lack a created timestamp)"
	}
	issueMetrics.Coverage = coverage
	prMetrics.Coverage = coverage
	return issueMetrics, prMetrics
}

func withinWindow(t time.Time, start, end time.Time) bool {
	if !start.IsZero() && t.Before(start) {
		return false
	}
	if !end.IsZero() && t.After(end) {
		return false
	}
	return true
}

func computeExternalMetrics(threads []corpus.Thread, start, end time.Time) ExternalContributorMetrics {
	out := ExternalContributorMetrics{}
	var unknown, known int
	for _, t := range threads {
		if t.Kind != corpus.ThreadKindPullRequest {
			continue
		}
		if !t.SourceCreatedAt.IsZero() && !withinWindow(t.SourceCreatedAt, start, end) {
			continue
		}
		if t.SourceCreatedAt.IsZero() && !start.IsZero() {
			continue
		}
		out.SampleSize++
		assoc := t.AuthorAssociation
		if assoc == "" {
			unknown++
			continue
		}
		known++
		if !isExternalAssociation(assoc) {
			continue
		}
		out.External++
		switch {
		case t.State == "open":
			out.Open++
		case t.Merged:
			out.Merged++
		default:
			out.ClosedUnmerged++
		}
	}
	out.Known = known
	switch {
	case out.SampleSize == 0:
		out.Coverage = "missing (no PRs in window)"
	case known == 0:
		out.Coverage = "missing (no author association)"
	case unknown > 0:
		out.Coverage = "partial (some PRs lack author association)"
	default:
		out.Coverage = "complete"
	}
	if out.Merged+out.ClosedUnmerged > 0 {
		out.MergeRate = float64(out.Merged) / float64(out.Merged+out.ClosedUnmerged)
	}
	return out
}

func isExternalAssociation(assoc string) bool {
	if assoc == "" {
		return false
	}
	switch strings.ToLower(assoc) {
	case "owner", "member", "collaborator", "mannequin":
		return false
	}
	return true
}

func computeCongestion(threads []corpus.Thread, now time.Time, window Window) CongestionMetrics {
	out := CongestionMetrics{Window: window}
	var ages []float64
	buckets := []struct {
		label string
		max   time.Duration
	}{
		{"< 7 days", 7 * 24 * time.Hour},
		{"7-30 days", 30 * 24 * time.Hour},
		{"> 30 days", 0},
	}
	bucketCounts := make([]int, len(buckets))
	var missingCreated int
	for _, t := range threads {
		if t.Kind != corpus.ThreadKindPullRequest || t.State != "open" {
			continue
		}
		out.OpenPRs++
		if t.SourceCreatedAt.IsZero() || t.SourceCreatedAt.After(now) {
			missingCreated++
			continue
		}
		age := now.Sub(t.SourceCreatedAt)
		hours := age.Hours()
		ages = append(ages, hours)
		out.SampleSize++
		placed := false
		for i, b := range buckets {
			if b.max == 0 || age < b.max {
				bucketCounts[i]++
				placed = true
				break
			}
		}
		if !placed {
			bucketCounts[len(bucketCounts)-1]++
		}
	}
	out.AgeBuckets = make([]AgeBucket, len(buckets))
	for i := range buckets {
		out.AgeBuckets[i] = AgeBucket{Label: buckets[i].label, Count: bucketCounts[i]}
	}
	if len(ages) > 0 {
		sort.Float64s(ages)
		out.MedianAge = percentile(ages, 0.5)
		out.P90Age = percentile(ages, 0.9)
		out.MaxAge = ages[len(ages)-1]
	}
	switch {
	case out.OpenPRs == 0:
		out.Coverage = "complete (no open PRs)"
	case missingCreated == 0:
		out.Coverage = "complete"
	default:
		out.Coverage = "partial (some open PRs lack a created timestamp)"
	}
	return out
}

func computeStaleSignals(ctx context.Context, c *corpus.Corpus, threads []corpus.Thread, now time.Time, threshold time.Duration, window Window) (StaleMetrics, error) {
	out := StaleMetrics{
		Window:    window,
		Threshold: threshold.Hours(),
	}
	for _, t := range threads {
		if t.Kind != corpus.ThreadKindPullRequest || t.State != "open" {
			continue
		}
		out.SampleSize++
		latest, eventCount, err := latestActivity(ctx, c, t)
		if err != nil {
			return out, err
		}
		if latest.IsZero() {
			out.MissingCoverageCount++
			continue
		}
		if eventCount == 0 {
			out.NoReviewOrCommentCount++
		}
		if now.Sub(latest) > threshold {
			out.StaleCount++
		} else {
			out.ActiveCount++
		}
	}
	switch {
	case out.SampleSize == 0:
		out.Coverage = "complete (no open PRs)"
	case out.MissingCoverageCount > 0:
		out.Coverage = "partial (some open PRs lack update/review timestamps)"
	default:
		out.Coverage = "complete"
	}
	return out, nil
}

func latestActivity(ctx context.Context, c *corpus.Corpus, t corpus.Thread) (time.Time, int, error) {
	var latest time.Time
	if !t.SourceUpdatedAt.IsZero() {
		latest = t.SourceUpdatedAt
	}
	var eventCount int
	facets := []string{facetIssueComments, facetPRReviews, facetPRReviewComments}
	for _, facet := range facets {
		obs, err := c.ListFacetObservations(ctx, t.RepositoryID, &t.ID, facet)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("list %s: %w", facet, err)
		}
		for _, o := range obs {
			events, err := parseFacetEvents(o.Payload, facet)
			if err != nil {
				return time.Time{}, 0, err
			}
			for _, e := range events {
				eventCount++
				if !e.Time.IsZero() && e.Time.After(latest) {
					latest = e.Time
				}
			}
		}
	}
	return latest, eventCount, nil
}

func computeResponseTimes(ctx context.Context, c *corpus.Corpus, threads []corpus.Thread, start, end time.Time, window Window) (ResponseTimeDistributions, error) {
	out := ResponseTimeDistributions{}
	issueSamples, issueCoverage, err := responseSamples(ctx, c, threads, start, end, corpus.ThreadKindIssue)
	if err != nil {
		return out, err
	}
	out.Issues = buildResponseMetric(issueSamples, window, issueCoverage, "issue_comments")

	prSamples, prCoverage, err := responseSamples(ctx, c, threads, start, end, corpus.ThreadKindPullRequest)
	if err != nil {
		return out, err
	}
	out.PullRequests = buildResponseMetric(prSamples, window, prCoverage, "issue_comments/pr_reviews/pr_review_comments")
	return out, nil
}

func responseSamples(ctx context.Context, c *corpus.Corpus, threads []corpus.Thread, start, end time.Time, kind string) ([]float64, string, error) {
	var samples []float64
	var withFacets, withoutFacets, noCreated int
	for _, t := range threads {
		if t.Kind != kind {
			continue
		}
		if t.SourceCreatedAt.IsZero() {
			noCreated++
			continue
		}
		if !withinWindow(t.SourceCreatedAt, start, end) {
			continue
		}
		first, hasFacet, err := firstResponse(ctx, c, t)
		if err != nil {
			return nil, "", err
		}
		if hasFacet {
			withFacets++
		} else {
			withoutFacets++
		}
		if !first.IsZero() {
			dur := first.Sub(t.SourceCreatedAt)
			if dur >= 0 {
				samples = append(samples, dur.Hours())
			}
		}
	}
	coverage := "complete"
	if noCreated > 0 || withoutFacets > 0 {
		coverage = "partial"
	}
	if withFacets == 0 && withoutFacets == 0 && noCreated == 0 {
		coverage = "missing (no threads in window)"
	}
	return samples, coverage, nil
}

func firstResponse(ctx context.Context, c *corpus.Corpus, t corpus.Thread) (time.Time, bool, error) {
	var earliest time.Time
	hasFacet := false
	facets := []string{facetIssueComments}
	if t.Kind == corpus.ThreadKindPullRequest {
		facets = append(facets, facetPRReviews, facetPRReviewComments)
	}
	for _, facet := range facets {
		obs, err := c.ListFacetObservations(ctx, t.RepositoryID, &t.ID, facet)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("list %s: %w", facet, err)
		}
		if len(obs) > 0 {
			hasFacet = true
		}
		for _, o := range obs {
			events, err := parseFacetEvents(o.Payload, facet)
			if err != nil {
				return time.Time{}, false, err
			}
			for _, e := range events {
				if e.Author == "" || e.Author == t.Author {
					continue
				}
				if !e.Time.IsZero() && (earliest.IsZero() || e.Time.Before(earliest)) {
					earliest = e.Time
				}
			}
		}
	}
	return earliest, hasFacet, nil
}

type facetEvent struct {
	ID     int64
	Author string
	Time   time.Time
}

func parseFacetEvents(payload, facet string) ([]facetEvent, error) {
	if payload == "" {
		return nil, nil
	}
	switch facet {
	case facetIssueComments, facetPRReviewComments:
		var items []github.IssueComment
		if err := json.Unmarshal([]byte(payload), &items); err != nil {
			return nil, fmt.Errorf("parse %s payload: %w", facet, err)
		}
		out := make([]facetEvent, 0, len(items))
		for _, item := range items {
			out = append(out, facetEvent{ID: item.ID, Author: item.Author, Time: item.CreatedAt})
		}
		return out, nil
	case facetPRReviews:
		var items []github.Review
		if err := json.Unmarshal([]byte(payload), &items); err != nil {
			return nil, fmt.Errorf("parse %s payload: %w", facet, err)
		}
		out := make([]facetEvent, 0, len(items))
		for _, item := range items {
			out = append(out, facetEvent{ID: item.ID, Author: item.Author, Time: item.SubmittedAt})
		}
		return out, nil
	default:
		return nil, nil
	}
}

func buildResponseMetric(samples []float64, window Window, coverage, source string) ResponseTimeMetric {
	m := ResponseTimeMetric{Window: window, Coverage: coverage, SampleSize: len(samples), Source: source}
	if len(samples) == 0 {
		return m
	}
	sort.Float64s(samples)
	m.Median = percentile(samples, 0.5)
	m.P90 = percentile(samples, 0.9)
	m.Min = samples[0]
	m.Max = samples[len(samples)-1]
	var sum float64
	for _, v := range samples {
		sum += v
	}
	m.Mean = sum / float64(len(samples))
	return m
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return sorted[lower]
	}
	frac := pos - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}
