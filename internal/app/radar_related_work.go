package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

const (
	maxRadarRelatedWork         = 50
	maxRadarEvidencePerRelation = 20
)

type rawRadarRelatedWork struct {
	reference relatedwork.Reference
	direction string
	evidence  radar.RelatedWorkEvidence
}

func radarPullRequestRelatedWork(ctx context.Context, c *corpus.Corpus, stored *corpus.Repository, ref domain.RepoRef, issues, pullRequests []corpus.Thread) (map[int][]radar.RelatedWork, bool, error) {
	issueNumbers := make(map[int]struct{}, len(issues))
	for _, issue := range issues {
		issueNumbers[issue.Number] = struct{}{}
	}
	out, byNumber, err := radarPullRequestTextRelationships(ctx, ref, issueNumbers, pullRequests)
	if err != nil {
		return nil, false, err
	}
	projected, projectedCapped, err := c.ListPullRequestIssueLinks(ctx, stored.ID, "open", radarPullRequestPopulation)
	if err != nil {
		return nil, false, fmt.Errorf("list authoritative closing-issue relationships: %w", err)
	}
	if err := appendRadarProjectedClosures(ctx, out, projected, byNumber, issueNumbers, ref); err != nil {
		return nil, false, err
	}
	for number, values := range out {
		normalized, _ := normalizeRadarRelatedWork(values, radarPullRequestPopulation)
		out[number] = normalized
	}
	return out, projectedCapped, nil
}

func radarPullRequestTextRelationships(ctx context.Context, ref domain.RepoRef, issueNumbers map[int]struct{}, pullRequests []corpus.Thread) (map[int][]radar.RelatedWork, map[int]corpus.Thread, error) {
	out := make(map[int][]radar.RelatedWork)
	byNumber := make(map[int]corpus.Thread, len(pullRequests))
	for _, pullRequest := range pullRequests {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		byNumber[pullRequest.Number] = pullRequest
		for _, linked := range relatedwork.Extract(pullRequest.Title+"\n"+pullRequest.Body, ref) {
			if !radarReferenceTargetsIssue(linked, ref, issueNumbers) {
				continue
			}
			relation := linked.Relation
			if relation == relatedwork.RelationExplicitReference {
				relation = relatedwork.RelationMentions
			}
			out[linked.Number] = append(out[linked.Number], radarPullRequestWork(ref, pullRequest, relation, "pull_request_text", pullRequest.SourceUpdatedAt))
		}
	}
	return out, byNumber, nil
}

func appendRadarProjectedClosures(ctx context.Context, out map[int][]radar.RelatedWork, projected []corpus.PullRequestIssueLinks, byNumber map[int]corpus.Thread, issueNumbers map[int]struct{}, ref domain.RepoRef) error {
	for _, links := range projected {
		if err := ctx.Err(); err != nil {
			return err
		}
		pullRequest, ok := byNumber[links.Number]
		if !ok || !links.Covered {
			continue
		}
		for _, value := range links.LinkedIssues {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, linked := range relatedwork.Extract(value, ref) {
				if radarReferenceTargetsIssue(linked, ref, issueNumbers) {
					out[linked.Number] = append(out[linked.Number], radarPullRequestWork(ref, pullRequest, relatedwork.RelationClaimsToClose, "github_closing_issue", links.SourceUpdatedAt))
				}
			}
		}
	}
	return nil
}

func radarReferenceTargetsIssue(linked relatedwork.Reference, ref domain.RepoRef, issueNumbers map[int]struct{}) bool {
	if !sameRepo(linked.Repo, ref) || linked.Kind == domain.PullRequestKind {
		return false
	}
	_, ok := issueNumbers[linked.Number]
	return ok
}

func radarPullRequestWork(ref domain.RepoRef, pullRequest corpus.Thread, relation, evidenceKind string, sourceAsOf time.Time) radar.RelatedWork {
	url := threadURL(ref, string(domain.PullRequestKind), pullRequest.Number)
	return radar.RelatedWork{
		Ref: fmt.Sprintf("pull_request:%s#%d", ref, pullRequest.Number), Kind: string(domain.PullRequestKind),
		Number: pullRequest.Number, Title: pullRequest.Title, State: pullRequest.State,
		Relation: relation, Direction: "inbound", URL: url,
		Evidence:        []radar.RelatedWorkEvidence{{Kind: evidenceKind, SourceURL: url, SourceAsOf: sourceAsOf}},
		SourceUpdatedAt: pullRequest.SourceUpdatedAt,
	}
}

type radarWorkAccumulator struct {
	repo         domain.RepoRef
	targetNumber int
	raw          []rawRadarRelatedWork
	distinct     map[string]struct{}
	priorities   map[string]int
	capped       bool
}

func newRadarWorkAccumulator(repo domain.RepoRef, targetNumber int) *radarWorkAccumulator {
	return &radarWorkAccumulator{
		repo: repo, targetNumber: targetNumber, distinct: map[string]struct{}{}, priorities: map[string]int{},
	}
}

func (a *radarWorkAccumulator) appendText(text, direction, evidenceKind, sourceURL string, sourceAsOf time.Time) {
	for _, reference := range relatedwork.Extract(text, a.repo) {
		a.append(reference, direction, radar.RelatedWorkEvidence{Kind: evidenceKind, SourceURL: sourceURL, SourceAsOf: sourceAsOf})
	}
}

func (a *radarWorkAccumulator) append(reference relatedwork.Reference, direction string, evidence radar.RelatedWorkEvidence) {
	if sameRepo(reference.Repo, a.repo) && reference.Number == a.targetNumber {
		return
	}
	if direction == "outbound" && reference.Relation == relatedwork.RelationClaimsToClose {
		reference.Relation = relatedwork.RelationExplicitReference
	}
	key := radarReferenceKey(reference)
	if _, ok := a.distinct[key]; !ok {
		if len(a.distinct) >= maxRadarRelatedWork {
			a.capped = true
			worst := a.leastPreferredKey()
			if !radarReferencePreferred(reference, key, a.priorities[worst], worst) {
				return
			}
			delete(a.distinct, worst)
			delete(a.priorities, worst)
			a.removeRawKey(worst)
		}
		a.distinct[key] = struct{}{}
	}
	a.priorities[key] = max(a.priorities[key], relatedwork.Priority(reference.Relation))
	if radarRawEvidenceCount(a.raw, key) >= maxRadarEvidencePerRelation {
		a.capped = true
		return
	}
	a.raw = append(a.raw, rawRadarRelatedWork{reference: reference, direction: direction, evidence: evidence})
}

func (a *radarWorkAccumulator) leastPreferredKey() string {
	worst := ""
	for key := range a.distinct {
		if worst == "" || a.priorities[key] < a.priorities[worst] || a.priorities[key] == a.priorities[worst] && key > worst {
			worst = key
		}
	}
	return worst
}

func (a *radarWorkAccumulator) removeRawKey(key string) {
	kept := a.raw[:0]
	for _, value := range a.raw {
		if radarReferenceKey(value.reference) != key {
			kept = append(kept, value)
		}
	}
	a.raw = kept
}

func radarReferencePreferred(reference relatedwork.Reference, key string, otherPriority int, otherKey string) bool {
	priority := relatedwork.Priority(reference.Relation)
	return priority > otherPriority || priority == otherPriority && key < otherKey
}

func radarIssueDiscussionAndRelatedWork(ctx context.Context, c *corpus.Corpus, stored *corpus.Repository, issue corpus.Thread, ref domain.RepoRef, now time.Time) (radar.DiscussionSummary, []radar.RelatedWork, bool, error) {
	issueURL := threadURL(ref, string(domain.IssueKind), issue.Number)
	accumulator := newRadarWorkAccumulator(ref, issue.Number)
	accumulator.appendText(issue.Title+"\n"+issue.Body, "outbound", "issue_text", issueURL, issue.SourceUpdatedAt)
	comments, err := readRadarIssueComments(ctx, c, stored.ID, issue.ID, accumulator)
	if err != nil {
		return radar.DiscussionSummary{}, nil, false, err
	}
	if err := readRadarIssueTimeline(ctx, c, stored.ID, issue.ID, issueURL, accumulator); err != nil {
		return radar.DiscussionSummary{}, nil, false, err
	}
	values, err := resolveRadarRelatedWork(ctx, c, accumulator.raw)
	if err != nil {
		return radar.DiscussionSummary{}, nil, false, err
	}
	values, normalizedCapped := normalizeRadarRelatedWork(values, maxRadarRelatedWork)
	return radar.SummarizeDiscussion(comments, now), values, accumulator.capped || normalizedCapped, nil
}

func readRadarIssueComments(ctx context.Context, c *corpus.Corpus, repoID, issueID int64, accumulator *radarWorkAccumulator) ([]radar.DiscussionComment, error) {
	commentObservations, commentPagesCapped, err := c.ListFacetObservationsBounded(ctx, repoID, &issueID, FacetIssueComments, maxHydrationPages)
	if err != nil {
		return nil, err
	}
	if commentPagesCapped {
		return nil, errors.New("stored issue comments exceed the hydration page bound")
	}
	comments := []radar.DiscussionComment{}
	for _, observation := range commentObservations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var values []github.IssueComment
		if err := json.Unmarshal([]byte(observation.Payload), &values); err != nil {
			return nil, fmt.Errorf("decode issue comment observation %d: %w", observation.ID, err)
		}
		for _, comment := range values {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			comments = append(comments, radar.DiscussionComment{
				Author: comment.Author, AuthorAssociation: comment.AuthorAssociation, Body: comment.Body,
				URL: comment.HTMLURL, CreatedAt: comment.CreatedAt,
			})
			sourceAsOf := comment.UpdatedAt
			if sourceAsOf.IsZero() {
				sourceAsOf = comment.CreatedAt
			}
			accumulator.appendText(comment.Body, "outbound", "issue_comment", comment.HTMLURL, sourceAsOf)
		}
	}
	return comments, nil
}

func readRadarIssueTimeline(ctx context.Context, c *corpus.Corpus, repoID, issueID int64, issueURL string, accumulator *radarWorkAccumulator) error {
	timelineObservations, timelinePagesCapped, err := c.ListFacetObservationsBounded(ctx, repoID, &issueID, FacetIssueTimeline, maxHydrationPages)
	if err != nil {
		return err
	}
	if timelinePagesCapped {
		return errors.New("stored issue timeline exceeds the hydration page bound")
	}
	for _, observation := range timelineObservations {
		if err := ctx.Err(); err != nil {
			return err
		}
		var values []github.IssueTimelineEvent
		if err := json.Unmarshal([]byte(observation.Payload), &values); err != nil {
			return fmt.Errorf("decode issue timeline observation %d: %w", observation.ID, err)
		}
		for _, event := range values {
			if err := ctx.Err(); err != nil {
				return err
			}
			reference, ok := radarTimelineReference(event, accumulator.repo)
			if !ok {
				continue
			}
			accumulator.append(reference, "inbound", radar.RelatedWorkEvidence{
				Kind: "issue_timeline", SourceURL: issueURL, SourceAsOf: observation.SourceUpdatedAt,
			})
		}
	}
	return nil
}

func radarTimelineReference(event github.IssueTimelineEvent, defaultRepo domain.RepoRef) (relatedwork.Reference, bool) {
	if event.Event != "cross-referenced" || event.SourceNumber <= 0 {
		return relatedwork.Reference{}, false
	}
	sourceRepo := domain.RepoRef{Owner: event.SourceOwner, Repo: event.SourceRepository}
	if sourceRepo.Owner == "" || sourceRepo.Repo == "" {
		sourceRepo = defaultRepo
	}
	kind := domain.ThreadKind("")
	if event.SourceIsPullRequest {
		kind = domain.PullRequestKind
	}
	return relatedwork.Reference{Repo: sourceRepo, Kind: kind, Number: event.SourceNumber, Relation: relatedwork.RelationCrossReference}, true
}

func resolveRadarRelatedWork(ctx context.Context, c *corpus.Corpus, raw []rawRadarRelatedWork) ([]radar.RelatedWork, error) {
	values := make([]radar.RelatedWork, 0, len(raw))
	for _, item := range raw {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resolved, err := resolveRadarReference(ctx, c, item.reference, item.direction, item.evidence)
		if err != nil {
			return nil, err
		}
		values = append(values, resolved)
	}
	return values, nil
}

func resolveRadarReference(ctx context.Context, c *corpus.Corpus, reference relatedwork.Reference, direction string, evidence radar.RelatedWorkEvidence) (radar.RelatedWork, error) {
	kind := reference.Kind
	state, title := "", ""
	sourceUpdatedAt := time.Time{}
	storedRepo, err := c.GetRepository(ctx, reference.Repo.Owner, reference.Repo.Repo)
	if err != nil {
		return radar.RelatedWork{}, fmt.Errorf("resolve related repository: %w", err)
	}
	if storedRepo != nil {
		thread, err := c.GetThreadByNumber(ctx, storedRepo.ID, reference.Number)
		if err != nil {
			return radar.RelatedWork{}, fmt.Errorf("resolve related thread: %w", err)
		}
		if thread != nil {
			kind = domain.ThreadKind(thread.Kind)
			state, title, sourceUpdatedAt = thread.State, thread.Title, thread.SourceUpdatedAt
		}
	}
	kindName := string(kind)
	if kindName == "" {
		kindName = "thread"
	}
	return radar.RelatedWork{
		Ref: fmt.Sprintf("%s:%s#%d", kindName, reference.Repo, reference.Number), Kind: kindName,
		Number: reference.Number, Title: title, State: state, Relation: reference.Relation, Direction: direction,
		URL: threadURL(reference.Repo, string(kind), reference.Number), Evidence: []radar.RelatedWorkEvidence{evidence}, SourceUpdatedAt: sourceUpdatedAt,
	}, nil
}

func normalizeRadarRelatedWork(values []radar.RelatedWork, limit int) ([]radar.RelatedWork, bool) {
	merged := map[string]radar.RelatedWork{}
	capped := false
	for _, value := range values {
		key := strings.ToLower(value.Ref)
		current, ok := merged[key]
		if !ok {
			if len(value.Evidence) > maxRadarEvidencePerRelation {
				capped = true
			}
			value.Evidence = normalizeRadarRelationshipEvidence(value.Evidence)
			merged[key] = value
			continue
		}
		if relatedwork.Priority(value.Relation) > relatedwork.Priority(current.Relation) {
			current.Relation, current.Direction = value.Relation, value.Direction
		}
		if value.Title != "" {
			current.Title = value.Title
		}
		if value.State != "" {
			current.State = value.State
		}
		if value.Kind != "thread" {
			current.Kind, current.Ref, current.URL = value.Kind, value.Ref, value.URL
		}
		if value.SourceUpdatedAt.After(current.SourceUpdatedAt) {
			current.SourceUpdatedAt = value.SourceUpdatedAt
		}
		combinedEvidence := append(current.Evidence, value.Evidence...)
		if len(combinedEvidence) > maxRadarEvidencePerRelation {
			capped = true
		}
		current.Evidence = normalizeRadarRelationshipEvidence(combinedEvidence)
		merged[key] = current
	}
	out := make([]radar.RelatedWork, 0, len(merged))
	for _, value := range merged {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := relatedwork.Priority(out[i].Relation), relatedwork.Priority(out[j].Relation)
		if left != right {
			return left > right
		}
		return out[i].Ref < out[j].Ref
	})
	if len(out) > limit {
		out = out[:limit]
		capped = true
	}
	return out, capped
}

func normalizeRadarRelationshipEvidence(values []radar.RelatedWorkEvidence) []radar.RelatedWorkEvidence {
	seen := map[string]radar.RelatedWorkEvidence{}
	for _, value := range values {
		key := value.Kind + "\x00" + value.SourceURL + "\x00" + value.SourceAsOf.UTC().Format(time.RFC3339Nano)
		seen[key] = value
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxRadarEvidencePerRelation {
		keys = keys[:maxRadarEvidencePerRelation]
	}
	out := make([]radar.RelatedWorkEvidence, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func radarReferenceKey(value relatedwork.Reference) string {
	return strings.ToLower(fmt.Sprintf("%s/%s:%s#%d", value.Repo.Owner, value.Repo.Repo, value.Kind, value.Number))
}

func radarRawEvidenceCount(values []rawRadarRelatedWork, key string) int {
	count := 0
	for _, value := range values {
		if radarReferenceKey(value.reference) == key {
			count++
		}
	}
	return count
}

func sameRepo(left, right domain.RepoRef) bool {
	return strings.EqualFold(left.Owner, right.Owner) && strings.EqualFold(left.Repo, right.Repo)
}
