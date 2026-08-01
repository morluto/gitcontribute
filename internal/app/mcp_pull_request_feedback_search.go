package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SearchPullRequestFeedback is an offline read over the repository feedback
// projection. Coverage state is returned independently from match count.
func (r *MCPReader) SearchPullRequestFeedback(ctx context.Context, in mcpcontract.SearchPullRequestFeedbackInput) (mcpcontract.SearchPullRequestFeedbackOutput, error) {
	ref := domain.RepoRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	createdAfter, err := parseFeedbackDate("created_after", in.CreatedAfter)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	createdBefore, err := parseFeedbackDate("created_before", in.CreatedBefore)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	updatedAfter, err := parseFeedbackDate("updated_after", in.UpdatedAfter)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	updatedBefore, err := parseFeedbackDate("updated_before", in.UpdatedBefore)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	if repo == nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, mcpcontract.Unavailable("repository_feedback_not_indexed", fmt.Sprintf("No pull-request feedback index exists for %s.", ref), mcpcontract.RecoveryAction(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: ref.Owner, Repo: ref.Repo}}))
	}
	page, err := c.SearchPullRequestFeedback(ctx, corpus.FeedbackSearchFilter{
		RepositoryID: repo.ID, FeedbackAuthor: in.FeedbackAuthor, PullRequestAuthor: in.PullRequestAuthor,
		State: in.State, Merged: in.Merged, ThreadState: in.ThreadState, Channel: in.Channel, Text: in.Text,
		CreatedAfter: createdAfter, CreatedBefore: createdBefore, UpdatedAfter: updatedAfter, UpdatedBefore: updatedBefore,
		Sort: in.Sort, Order: in.Order, Limit: in.Limit, Cursor: in.Cursor,
	})
	if err != nil {
		if errors.Is(err, corpus.ErrProjectionStale) {
			return mcpcontract.SearchPullRequestFeedbackOutput{}, mcpcontract.Unavailable("feedback_projection_stale", "The normalized feedback projection is missing or stale. Continue the repository feedback index job, then retry this offline search.", mcpcontract.RecoveryAction(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: ref.Owner, Repo: ref.Repo}, Channels: []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}, ThreadState: "all"}))
		}
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	out := mcpcontract.SearchPullRequestFeedbackOutput{
		Status: "complete", Coverage: page.Coverage.Status, Total: page.Total, Truncated: page.Truncated,
		NextCursor: page.NextCursor, SnapshotToken: snapshotIdentity(in.SnapshotToken, revision),
		Projection: corpus.ProjectionVersionPullRequestFeedbackFTS, IncompletePRs: page.Coverage.IncompletePRs,
		Matches: make([]mcpcontract.PullRequestFeedbackMatch, 0, len(page.Items)),
	}
	for _, item := range page.Items {
		merged := (*bool)(nil)
		if item.PullRequestMergedKnown {
			value := item.PullRequestMerged
			merged = &value
		}
		resolved := (*bool)(nil)
		resolutionState := "unknown"
		if item.ResolvedKnown {
			value := item.Resolved
			resolved = &value
			if value {
				resolutionState = "resolved"
			} else {
				resolutionState = "unresolved"
			}
		}
		pr := mcpcontract.ThreadRef{Owner: ref.Owner, Repo: ref.Repo, Kind: "pull_request", Number: item.PullRequestNumber}
		prReference := fmt.Sprintf("%s/%s#%d", ref.Owner, ref.Repo, item.PullRequestNumber)
		threadReference := fmt.Sprintf("gitcontribute://pull-request-feedback/%s/%s/%d", ref.Owner, ref.Repo, item.PullRequestNumber)
		out.Matches = append(out.Matches, mcpcontract.PullRequestFeedbackMatch{
			Repository: in.Repository, PullRequest: pr, PullRequestReference: prReference,
			PullRequestAuthor: item.PullRequestAuthor, PullRequestState: item.PullRequestState, Merged: merged,
			Channel: item.Channel, FeedbackID: item.FeedbackID, FeedbackNodeID: item.FeedbackNodeID, ThreadID: item.ThreadExternalID,
			ThreadReference:  threadReference,
			CommentReference: fmt.Sprintf("gitcontribute://pull-request-feedback/%s/%s/%d/%s/%s", ref.Owner, ref.Repo, item.PullRequestNumber, url.PathEscape(item.Channel), url.PathEscape(item.FeedbackID)),
			InReplyToID:      item.InReplyToID, FeedbackAuthor: item.Author, ReviewState: item.ReviewState, Body: compactFeedbackBody(item.Body), Path: item.Path, Line: item.Line,
			StartLine: item.StartLine, Side: item.Side, StartSide: item.StartSide, Outdated: item.Outdated, Resolved: resolved, ResolutionState: resolutionState, ResolvedBy: item.ResolvedBy, CreatedAt: formatTime(item.CreatedAt),
			UpdatedAt: formatTime(item.UpdatedAt), HeadSHA: item.HeadSHA, SourceObservationID: item.SourceObservationID,
		})
	}
	unknownMergeState := len(page.UnknownMergePullRequests) > 0
	if !unknownMergeState {
		for _, item := range page.Items {
			if !item.PullRequestMergedKnown {
				unknownMergeState = true
				break
			}
		}
	}
	if page.Coverage.Status != "complete" || unknownMergeState {
		out.Status = "partial"
		out.Recovery = feedbackSearchRecovery(ctx, c, repo.ID, ref, in, page)
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.SearchPullRequestFeedbackOutput{}, err
	}
	return out, nil
}

func parseFeedbackDate(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return parsed, nil
}

func compactFeedbackBody(value string) string {
	const max = 512
	if len(value) <= max {
		return value
	}
	end := max
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + " …"
}

func feedbackSearchRecovery(ctx context.Context, c *corpus.Corpus, repositoryID int64, ref domain.RepoRef, in mcpcontract.SearchPullRequestFeedbackInput, page corpus.FeedbackSearchPage) *mcpcontract.RecoveryPlan {
	if !page.Coverage.DiscoveryComplete {
		return recoveryPlan("feedback_discovery_incomplete", "Discovery is incomplete; continue the repository feedback index job before treating an empty result as absence.", mcpcontract.RecoveryAction(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: ref.Owner, Repo: ref.Repo}, Channels: []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}, ThreadState: "all"}))
	}
	channels := []string{in.Channel}
	if in.Channel == "" {
		channels = []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}
	}
	if page.Coverage.IncompletePRs > 0 {
		threadState := in.ThreadState
		if threadState == "" {
			threadState = "all"
		}
		threads, err := c.ListPullRequestsWithIncompleteFeedback(ctx, repositoryID, channels, threadState, 50)
		if err == nil && len(threads) > 0 {
			refs := make([]mcpcontract.ThreadRef, 0, len(threads))
			for _, thread := range threads {
				refs = append(refs, mcpcontract.ThreadRef{Owner: ref.Owner, Repo: ref.Repo, Kind: "pull_request", Number: thread.Number})
			}
			return recoveryPlan("feedback_facet_incomplete", "Some pull-request feedback facets are incomplete; retry the exact feedback synchronization, then reread this search.", mcpcontract.RecoveryAction(mcpcontract.SyncPullRequestFeedbackInput{PullRequests: refs, Channels: channels, ThreadState: "all", MaxItemsPerChannel: 1000, MaxRequests: 1000}))
		}
	}
	unknown := make([]mcpcontract.ThreadRef, 0, len(page.UnknownMergePullRequests)+len(page.Items))
	for _, number := range page.UnknownMergePullRequests {
		unknown = append(unknown, mcpcontract.ThreadRef{Owner: ref.Owner, Repo: ref.Repo, Kind: "pull_request", Number: number})
	}
	for _, item := range page.Items {
		if item.PullRequestMergedKnown {
			continue
		}
		unknown = append(unknown, mcpcontract.ThreadRef{Owner: ref.Owner, Repo: ref.Repo, Kind: "pull_request", Number: item.PullRequestNumber})
	}
	if len(unknown) > 0 {
		return recoveryPlan("merge_state_unknown", "Some matching pull requests have no observed merge state; refresh the exact PR-details facet before filtering on merge state.", mcpcontract.RecoveryAction(mcpcontract.HydrateThreadsInput{Threads: uniqueThreadRefs(unknown), Facets: []string{facets.PRDetails}, MaxPages: 1}))
	}
	return recoveryPlan("feedback_coverage_partial", "Feedback coverage is partial; continue indexing or retry the returned exact synchronization before treating missing feedback as absence.", mcpcontract.RecoveryAction(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: ref.Owner, Repo: ref.Repo}}))
}

func uniqueThreadRefs(values []mcpcontract.ThreadRef) []mcpcontract.ThreadRef {
	seen := make(map[string]struct{}, len(values))
	out := make([]mcpcontract.ThreadRef, 0, len(values))
	for _, value := range values {
		key := fmt.Sprintf("%s/%s#%d", value.Owner, value.Repo, value.Number)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
