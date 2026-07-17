package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/health"
	"github.com/morluto/gitcontribute/internal/research"
)

const (
	maxResearchDiscussionItems = 200
	maxResearchFacetPages      = 3
	maxResearchOpenPRScan      = 500
	maxResearchRelatedThreads  = 50
	maxResearchCodeHits        = 10
	researchCodeHitsPerTerm    = 5
)

var _ research.Reader = (*corpusReader)(nil)

func (r *corpusReader) ReadResearchThread(ctx context.Context, requested research.ThreadRef) (research.ThreadEvidence, error) {
	c, storedRepo, err := r.researchCorpusRepo(ctx, requested.Repo)
	if err != nil {
		return research.ThreadEvidence{}, err
	}
	thread, err := c.GetThreadByNumber(ctx, storedRepo.ID, requested.Number)
	if err != nil {
		return research.ThreadEvidence{}, fmt.Errorf("get thread: %w", err)
	}
	if thread == nil {
		return research.ThreadEvidence{}, fmt.Errorf("%w: %s#%d", research.ErrThreadNotFound, requested.Repo, requested.Number)
	}
	storedKind := domain.ThreadKind(thread.Kind)
	if requested.Kind != "" && requested.Kind != storedKind {
		return research.ThreadEvidence{}, research.KindMismatchError(requested.Kind, storedKind)
	}
	resolved := research.ThreadRef{Repo: requested.Repo, Kind: storedKind, Number: requested.Number}
	source, err := researchThreadSource(ctx, c, resolved, thread)
	if err != nil {
		return research.ThreadEvidence{}, err
	}
	evidence := research.ThreadEvidence{Thread: research.ThreadSnapshot{
		Ref: resolved, Title: thread.Title, Body: thread.Body, Author: thread.Author,
		AuthorAssociation: thread.AuthorAssociation, State: thread.State, StateReason: thread.StateReason,
		Labels: append([]string{}, thread.Labels...), Assignees: append([]string{}, thread.Assignees...),
		Draft: thread.Draft, Locked: thread.Locked, Milestone: thread.Milestone, Merged: thread.Merged,
		CreatedAt: thread.SourceCreatedAt, UpdatedAt: thread.SourceUpdatedAt, ClosedAt: thread.ClosedAt,
		MergedAt: thread.MergedAt, Source: source,
	}}

	for _, facet := range researchFacets(storedKind) {
		coverage, items, truncated, err := readResearchFacet(ctx, c, storedRepo.ID, thread.ID, resolved, facet)
		if err != nil {
			return research.ThreadEvidence{}, err
		}
		remaining := maxResearchDiscussionItems - len(evidence.Discussion)
		if remaining <= 0 {
			if len(items) > 0 {
				evidence.Truncated = true
				truncated = true
			}
			coverage.Truncated = truncated
			evidence.Coverage = append(evidence.Coverage, coverage)
			evidence.Truncated = evidence.Truncated || truncated
			continue
		}
		if len(items) > remaining {
			items = items[:remaining]
			truncated = true
		}
		evidence.Discussion = append(evidence.Discussion, items...)
		coverage.Truncated = truncated
		evidence.Coverage = append(evidence.Coverage, coverage)
		evidence.Truncated = evidence.Truncated || truncated
	}
	sort.SliceStable(evidence.Discussion, func(i, j int) bool {
		left, right := researchDiscussionTime(evidence.Discussion[i]), researchDiscussionTime(evidence.Discussion[j])
		if !left.Equal(right) {
			return left.Before(right)
		}
		if evidence.Discussion[i].Kind != evidence.Discussion[j].Kind {
			return evidence.Discussion[i].Kind < evidence.Discussion[j].Kind
		}
		return evidence.Discussion[i].ID < evidence.Discussion[j].ID
	})
	return evidence, nil
}

func (r *corpusReader) ReadResearchRelationships(ctx context.Context, ref research.ThreadRef, explicit []research.Reference) (research.RelationshipEvidence, error) {
	c, storedRepo, err := r.researchCorpusRepo(ctx, ref.Repo)
	if err != nil {
		return research.RelationshipEvidence{}, err
	}
	result := research.RelationshipEvidence{DuplicateThreads: []research.RelatedThread{}, PullRequests: []research.RelatedThread{}}
	result.Sources = append(result.Sources, research.SourceRef{
		Source: "local:duplicate-clusters", URL: fmt.Sprintf("local://clusters/%s/%s/%d", ref.Repo, ref.Kind, ref.Number), AsOf: storedRepo.SourceUpdatedAt,
	})
	if err := appendExplicitResearchRelations(ctx, c, explicit, &result); err != nil {
		return research.RelationshipEvidence{}, err
	}
	if err := appendClusterResearchRelations(ctx, c, ref, &result); err != nil {
		return research.RelationshipEvidence{}, err
	}
	if err := appendOpenPRResearchRelations(ctx, c, storedRepo, ref, &result); err != nil {
		return research.RelationshipEvidence{}, err
	}

	result.DuplicateThreads = normalizeResearchRelated(result.DuplicateThreads, maxResearchRelatedThreads, &result.DuplicateCapped)
	result.PullRequests = normalizeResearchRelated(result.PullRequests, maxResearchRelatedThreads, &result.PullRequestCapped)
	result.Sources = normalizeResearchSources(result.Sources)
	return result, nil
}

func appendExplicitResearchRelations(ctx context.Context, c *corpus.Corpus, explicit []research.Reference, result *research.RelationshipEvidence) error {
	for _, candidate := range explicit {
		related, err := resolveResearchReference(ctx, c, candidate)
		if err != nil {
			return err
		}
		related.Relation = "explicit_reference"
		related.Basis = "stored source text explicitly references this thread"
		result.DuplicateThreads = append(result.DuplicateThreads, related)
		result.Sources = append(result.Sources, candidate.Source)
		if related.Kind == string(domain.PullRequestKind) && strings.EqualFold(related.State, "open") {
			linked := related
			linked.Relation = "outbound_reference"
			result.PullRequests = append(result.PullRequests, linked)
		}
	}
	return nil
}

func appendClusterResearchRelations(ctx context.Context, c *corpus.Corpus, ref research.ThreadRef, result *research.RelationshipEvidence) error {
	cluster, err := c.Clustering().GetClusterForMember(ctx, clustering.MemberRef{
		Owner: ref.Repo.Owner, Repo: ref.Repo.Repo, Kind: string(ref.Kind), Number: ref.Number,
	})
	if err != nil {
		return fmt.Errorf("get duplicate cluster: %w", err)
	}
	if cluster == nil {
		return nil
	}
	clusterSource := research.SourceRef{
		Source: "local:duplicate-cluster", URL: "local://clusters/" + cluster.StableID,
		ObservedAt: cluster.UpdatedAt, AsOf: cluster.WindowEnd,
	}
	result.ClusterID = cluster.StableID
	result.Canonical = researchClusterRef(cluster.Canonical)
	result.Sources = append(result.Sources, clusterSource)
	for _, member := range cluster.Members {
		if !member.Included || researchMemberIsTarget(member.Ref, ref) {
			continue
		}
		result.DuplicateThreads = append(result.DuplicateThreads, research.RelatedThread{
			Ref: researchClusterRef(member.Ref), Kind: member.Ref.Kind, Number: member.Ref.Number,
			Title: member.Title, State: member.State, Relation: "cluster_candidate",
			Basis: member.Reason, URL: researchMemberURL(member.Ref), Source: clusterSource,
		})
		if len(result.DuplicateThreads) > maxResearchRelatedThreads {
			result.DuplicateCapped = true
			break
		}
	}
	return nil
}

func appendOpenPRResearchRelations(ctx context.Context, c *corpus.Corpus, storedRepo *corpus.Repository, ref research.ThreadRef, result *research.RelationshipEvidence) error {
	openPRs, err := c.ListThreadsFiltered(ctx, storedRepo.ID, corpus.ThreadKindPullRequest, "open", maxResearchOpenPRScan+1)
	if err != nil {
		return fmt.Errorf("list open pull requests: %w", err)
	}
	if len(openPRs) > maxResearchOpenPRScan {
		openPRs = openPRs[:maxResearchOpenPRScan]
		result.PullRequestCapped = true
	}
	prAsOf := storedRepo.SourceUpdatedAt
	for _, pullRequest := range openPRs {
		if pullRequest.SourceUpdatedAt.After(prAsOf) {
			prAsOf = pullRequest.SourceUpdatedAt
		}
		if pullRequest.Number == ref.Number && ref.Kind == domain.PullRequestKind {
			continue
		}
		if !researchTextReferences(pullRequest.Title+"\n"+pullRequest.Body, ref) {
			continue
		}
		relation := "mentions"
		basis := "open pull request explicitly references the target"
		if researchPRCloses(pullRequest.Title+"\n"+pullRequest.Body, ref) {
			relation = "claims_to_close"
			basis = "open pull request uses a closing keyword for the target"
		}
		source := research.SourceRef{
			Source: "github:rest", URL: fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", ref.Repo, pullRequest.Number),
			ObservedAt: pullRequest.UpdatedAt, AsOf: pullRequest.SourceUpdatedAt,
		}
		result.PullRequests = append(result.PullRequests, research.RelatedThread{
			Ref: fmt.Sprintf("pull_request:%s#%d", ref.Repo, pullRequest.Number), Kind: corpus.ThreadKindPullRequest,
			Number: pullRequest.Number, Title: pullRequest.Title, State: pullRequest.State,
			Relation: relation, Basis: basis, URL: fmt.Sprintf("https://github.com/%s/pull/%d", ref.Repo, pullRequest.Number), Source: source,
		})
	}
	result.Sources = append(result.Sources, research.SourceRef{
		Source: "github:rest", URL: fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=open", ref.Repo), AsOf: prAsOf,
	})
	return nil
}

func (r *corpusReader) ReadResearchCode(ctx context.Context, repo domain.RepoRef, terms []string) (research.CodeEvidence, error) {
	c, _, err := r.researchCorpusRepo(ctx, repo)
	if err != nil {
		return research.CodeEvidence{}, err
	}
	snapshot, err := c.LatestCodeSnapshot(ctx, repo)
	if err != nil {
		return research.CodeEvidence{}, fmt.Errorf("latest code snapshot: %w", err)
	}
	result := research.CodeEvidence{Queries: append([]string{}, terms...), Hits: []research.CodeHit{}}
	if snapshot == nil {
		return result, nil
	}
	result.Present = true
	result.CommitSHA = snapshot.CommitSHA
	result.Source = research.SourceRef{
		Source: "local:code-index", URL: fmt.Sprintf("https://github.com/%s/tree/%s", repo, snapshot.CommitSHA),
		CommitSHA: snapshot.CommitSHA, ObservedAt: snapshot.CreatedAt, AsOf: snapshot.CreatedAt,
	}
	seen := map[string]struct{}{}
	for _, term := range terms {
		matches, err := c.SearchCode(ctx, term, repo, researchCodeHitsPerTerm)
		if err != nil {
			return research.CodeEvidence{}, fmt.Errorf("search code for %q: %w", term, err)
		}
		if len(matches) == researchCodeHitsPerTerm {
			result.Truncated = true
		}
		for _, match := range matches {
			if _, ok := seen[match.Path]; ok {
				continue
			}
			seen[match.Path] = struct{}{}
			source := research.SourceRef{
				Source: "local:code-index", URL: fmt.Sprintf("https://github.com/%s/blob/%s/%s", repo, match.Commit, match.Path),
				CommitSHA: match.Commit, ObservedAt: match.SnapshotCreatedAt, AsOf: match.SnapshotCreatedAt,
			}
			result.Hits = append(result.Hits, research.CodeHit{
				Path: match.Path, Language: match.Language, CommitSHA: match.Commit, MatchedTerm: term, Source: source,
			})
			if len(result.Hits) == maxResearchCodeHits {
				result.Truncated = true
				break
			}
		}
		if len(result.Hits) == maxResearchCodeHits {
			break
		}
	}
	sort.Slice(result.Hits, func(i, j int) bool {
		if result.Hits[i].Path != result.Hits[j].Path {
			return result.Hits[i].Path < result.Hits[j].Path
		}
		return result.Hits[i].MatchedTerm < result.Hits[j].MatchedTerm
	})
	return result, nil
}

func (r *corpusReader) ReadResearchHealth(ctx context.Context, repo domain.RepoRef) (research.HealthEvidence, error) {
	c, storedRepo, err := r.researchCorpusRepo(ctx, repo)
	if err != nil {
		return research.HealthEvidence{}, err
	}
	report, err := health.Compute(ctx, c, storedRepo.ID, health.Options{Now: r.s.now(), StaleThreshold: 14 * 24 * time.Hour})
	if err != nil {
		return research.HealthEvidence{}, fmt.Errorf("compute health: %w", err)
	}
	healthAsOf := storedRepo.SourceUpdatedAt
	threads, err := c.ListThreads(ctx, storedRepo.ID, "", 1)
	if err != nil {
		return research.HealthEvidence{}, fmt.Errorf("read health source time: %w", err)
	}
	if len(threads) > 0 && threads[0].SourceUpdatedAt.After(healthAsOf) {
		healthAsOf = threads[0].SourceUpdatedAt
	}
	source := research.SourceRef{
		Source: "local:health", URL: "local://health/" + repo.String(), ObservedAt: report.GeneratedAt, AsOf: healthAsOf,
	}
	return research.HealthEvidence{
		Available: true, Archived: report.Repository.Archived, OpenIssues: report.Issues.Open,
		OpenPullRequests: report.PullRequests.Open, ExternalPRMergeRate: report.External.MergeRate,
		ExternalPRSampleSize:           report.External.SampleSize,
		IssueResponseMedianHours:       report.Response.Issues.Median,
		PullRequestResponseMedianHours: report.Response.PullRequests.Median,
		IssueResponseSampleSize:        report.Response.Issues.SampleSize,
		PullRequestResponseSampleSize:  report.Response.PullRequests.SampleSize,
		ThreadSampleSize:               report.Coverage.ThreadsSampleSize, ThreadsTruncated: report.Coverage.ThreadsTruncated,
		Sources: []research.SourceRef{source}, UnknownReason: researchHealthCoverageReason(report),
	}, nil
}

func researchHealthCoverageReason(report *health.Report) string {
	coverages := []struct {
		name  string
		value string
	}{
		{"issue metrics", report.Issues.Coverage},
		{"pull-request metrics", report.PullRequests.Coverage},
		{"external pull-request metrics", report.External.Coverage},
		{"issue response metrics", report.Response.Issues.Coverage},
		{"pull-request response metrics", report.Response.PullRequests.Coverage},
	}
	reasons := make([]string, 0, len(coverages))
	for _, coverage := range coverages {
		value := strings.TrimSpace(coverage.value)
		if strings.HasPrefix(strings.ToLower(value), "complete") {
			continue
		}
		if value == "" {
			value = "unknown"
		}
		reasons = append(reasons, coverage.name+" coverage is "+value)
	}
	return strings.Join(reasons, "; ")
}

func (r *corpusReader) researchCorpusRepo(ctx context.Context, ref domain.RepoRef) (*corpus.Corpus, *corpus.Repository, error) {
	c, err := r.s.openCorpus(ctx)
	if err != nil {
		return nil, nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return nil, nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}
	return c, repo, nil
}

func researchThreadSource(ctx context.Context, c *corpus.Corpus, ref research.ThreadRef, thread *corpus.Thread) (research.SourceRef, error) {
	observation, err := c.LatestThreadObservation(ctx, thread.ID)
	if err != nil {
		return research.SourceRef{}, fmt.Errorf("latest thread observation: %w", err)
	}
	observedAt := thread.UpdatedAt
	if observation != nil {
		observedAt = observation.ObservedAt
	}
	return research.SourceRef{
		Source: "github:rest", URL: fmt.Sprintf("https://api.github.com/repos/%s/issues/%d", ref.Repo, ref.Number),
		ObservedAt: observedAt, AsOf: thread.SourceUpdatedAt,
	}, nil
}

func researchFacets(kind domain.ThreadKind) []string {
	if kind == domain.PullRequestKind {
		return []string{FacetIssueComments, FacetPRDetails, FacetPRReviews, FacetPRReviewComments}
	}
	return []string{FacetIssueComments}
}

func readResearchFacet(ctx context.Context, c *corpus.Corpus, repoID, threadID int64, ref research.ThreadRef, facet string) (research.FacetCoverage, []research.DiscussionItem, bool, error) {
	coverage, err := c.GetCoverage(ctx, repoID, &threadID, facet)
	if err != nil {
		return research.FacetCoverage{}, nil, false, fmt.Errorf("get %s coverage: %w", facet, err)
	}
	if coverage == nil {
		return research.FacetCoverage{Facet: facet}, nil, false, nil
	}
	source := research.SourceRef{
		Source: "github:rest", URL: researchFacetURL(ref, facet), ObservedAt: coverage.UpdatedAt, AsOf: coverage.SourceUpdatedAt,
	}
	observations, pagesCapped, err := c.ListFacetObservationsBounded(ctx, repoID, &threadID, facet, maxResearchFacetPages)
	if err != nil {
		return research.FacetCoverage{}, nil, false, fmt.Errorf("list %s observations: %w", facet, err)
	}
	items := []research.DiscussionItem{}
	truncated := pagesCapped
	for observationIndex, observation := range observations {
		decoded, err := decodeResearchFacet(observation, ref, facet)
		if err != nil {
			return research.FacetCoverage{}, nil, false, err
		}
		remaining := maxResearchDiscussionItems - len(items)
		if remaining <= 0 {
			truncated = truncated || len(decoded) > 0 || observationIndex < len(observations)-1
			break
		}
		if len(decoded) > remaining {
			decoded = decoded[:remaining]
			truncated = true
		}
		items = append(items, decoded...)
	}
	count := len(items)
	if facet == FacetPRDetails && len(observations) > 0 {
		count = 1
	}
	return research.FacetCoverage{
		Facet: facet, Present: true, Complete: coverage.Complete, AsOf: coverage.SourceUpdatedAt, Count: count, Source: source,
	}, items, truncated, nil
}

func decodeResearchFacet(observation corpus.FacetObservation, ref research.ThreadRef, facet string) ([]research.DiscussionItem, error) {
	sourceFor := func(url string) research.SourceRef {
		if url == "" {
			url = researchFacetURL(ref, facet)
		}
		return research.SourceRef{Source: "github:rest", URL: url, ObservedAt: observation.ObservedAt, AsOf: observation.SourceUpdatedAt}
	}
	switch facet {
	case FacetIssueComments:
		var values []github.IssueComment
		if err := json.Unmarshal([]byte(observation.Payload), &values); err != nil {
			return nil, fmt.Errorf("parse %s observation: %w", facet, err)
		}
		out := make([]research.DiscussionItem, 0, len(values))
		for _, value := range values {
			out = append(out, research.DiscussionItem{
				ID: value.ID, Kind: "issue_comment", Body: value.Body, Author: value.Author,
				AuthorAssociation: value.AuthorAssociation, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
				Source: sourceFor(value.HTMLURL),
			})
		}
		return out, nil
	case FacetPRReviews:
		var values []github.Review
		if err := json.Unmarshal([]byte(observation.Payload), &values); err != nil {
			return nil, fmt.Errorf("parse %s observation: %w", facet, err)
		}
		out := make([]research.DiscussionItem, 0, len(values))
		for _, value := range values {
			out = append(out, research.DiscussionItem{
				ID: value.ID, Kind: "review", Body: value.Body, Author: value.Author,
				AuthorAssociation: value.AuthorAssociation, State: value.State, CreatedAt: value.SubmittedAt,
				Source: sourceFor(value.HTMLURL),
			})
		}
		return out, nil
	case FacetPRReviewComments:
		var values []github.ReviewComment
		if err := json.Unmarshal([]byte(observation.Payload), &values); err != nil {
			return nil, fmt.Errorf("parse %s observation: %w", facet, err)
		}
		out := make([]research.DiscussionItem, 0, len(values))
		for _, value := range values {
			out = append(out, research.DiscussionItem{
				ID: value.ID, Kind: "review_comment", Body: value.Body, Author: value.Author,
				AuthorAssociation: value.AuthorAssociation, Path: value.Path, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
				Source: sourceFor(value.HTMLURL),
			})
		}
		return out, nil
	case FacetPRDetails:
		if observation.Payload == "" {
			return nil, nil
		}
		var value github.PullRequestDetails
		if err := json.Unmarshal([]byte(observation.Payload), &value); err != nil {
			return nil, fmt.Errorf("parse %s observation: %w", facet, err)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported research facet %q", facet)
	}
}

func researchFacetURL(ref research.ThreadRef, facet string) string {
	base := fmt.Sprintf("https://api.github.com/repos/%s", ref.Repo)
	switch facet {
	case FacetIssueComments:
		return fmt.Sprintf("%s/issues/%d/comments", base, ref.Number)
	case FacetPRDetails:
		return fmt.Sprintf("%s/pulls/%d", base, ref.Number)
	case FacetPRReviews:
		return fmt.Sprintf("%s/pulls/%d/reviews", base, ref.Number)
	case FacetPRReviewComments:
		return fmt.Sprintf("%s/pulls/%d/comments", base, ref.Number)
	default:
		return base
	}
}

func resolveResearchReference(ctx context.Context, c *corpus.Corpus, candidate research.Reference) (research.RelatedThread, error) {
	kind := candidate.Kind
	state, title := "", ""
	repo, err := c.GetRepository(ctx, candidate.Repo.Owner, candidate.Repo.Repo)
	if err != nil {
		return research.RelatedThread{}, fmt.Errorf("resolve referenced repository: %w", err)
	}
	if repo != nil {
		thread, err := c.GetThreadByNumber(ctx, repo.ID, candidate.Number)
		if err != nil {
			return research.RelatedThread{}, fmt.Errorf("resolve referenced thread: %w", err)
		}
		if thread != nil {
			kind = domain.ThreadKind(thread.Kind)
			state, title = thread.State, thread.Title
		}
	}
	resolved := research.ThreadRef{Repo: candidate.Repo, Kind: kind, Number: candidate.Number}
	return research.RelatedThread{
		Ref: resolved.String(), Kind: string(kind), Number: candidate.Number, Title: title, State: state,
		URL: researchReferenceURL(resolved), Source: candidate.Source,
	}, nil
}

func researchReferenceURL(ref research.ThreadRef) string {
	segment := "issues"
	if ref.Kind == domain.PullRequestKind {
		segment = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", ref.Repo, segment, ref.Number)
}

func researchTextReferences(text string, target research.ThreadRef) bool {
	for _, ref := range clustering.ExtractRefs(text, target.Repo) {
		if strings.EqualFold(ref.Owner, target.Repo.Owner) && strings.EqualFold(ref.Repo, target.Repo.Repo) && ref.Number == target.Number {
			return true
		}
	}
	return false
}

func researchPRCloses(text string, target research.ThreadRef) bool {
	repo := regexp.QuoteMeta(target.Repo.String())
	number := regexp.QuoteMeta(strconv.Itoa(target.Number))
	pattern := regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:` + repo + `)?#` + number + `\b`)
	return pattern.MatchString(text)
}

func normalizeResearchRelated(values []research.RelatedThread, limit int, capped *bool) []research.RelatedThread {
	seen := map[string]research.RelatedThread{}
	for _, value := range values {
		key := strings.ToLower(value.Ref)
		previous, ok := seen[key]
		if !ok || researchRelationPriority(value.Relation) > researchRelationPriority(previous.Relation) {
			seen[key] = value
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
		*capped = true
	}
	out := make([]research.RelatedThread, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func researchRelationPriority(relation string) int {
	switch relation {
	case "claims_to_close":
		return 5
	case "cluster_candidate":
		return 4
	case "mentions":
		return 3
	case "outbound_reference":
		return 2
	case "explicit_reference":
		return 1
	default:
		return 0
	}
}

func normalizeResearchSources(values []research.SourceRef) []research.SourceRef {
	seen := map[string]research.SourceRef{}
	for _, value := range values {
		if value.Source == "" && value.URL == "" {
			continue
		}
		key := value.Source + "|" + value.URL + "|" + value.CommitSHA
		if _, ok := seen[key]; !ok {
			seen[key] = value
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]research.SourceRef, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func researchMemberIsTarget(member clustering.MemberRef, target research.ThreadRef) bool {
	return strings.EqualFold(member.Owner, target.Repo.Owner) && strings.EqualFold(member.Repo, target.Repo.Repo) && member.Kind == string(target.Kind) && member.Number == target.Number
}

func researchClusterRef(ref clustering.MemberRef) string {
	return fmt.Sprintf("%s:%s/%s#%d", ref.Kind, ref.Owner, ref.Repo, ref.Number)
}

func researchMemberURL(ref clustering.MemberRef) string {
	segment := "issues"
	if ref.Kind == corpus.ThreadKindPullRequest {
		segment = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%s/%d", ref.Owner, ref.Repo, segment, ref.Number)
}

func researchDiscussionTime(item research.DiscussionItem) time.Time {
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.UpdatedAt
}
