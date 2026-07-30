package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v89/github"
)

var ErrRequestBudgetExhausted = errors.New("github request budget exhausted")

type RequestBudget struct {
	mu        sync.Mutex
	limit     int
	completed int
}

func NewRequestBudget(limit int) *RequestBudget {
	return &RequestBudget{limit: limit}
}

func (b *RequestBudget) Take() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.completed >= b.limit {
		return ErrRequestBudgetExhausted
	}
	b.completed++
	return nil
}

func (b *RequestBudget) Completed() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.completed
}

type PullRequestFeedbackOptions struct {
	Channels           []string
	ThreadState        string
	MaxItemsPerChannel int
}

type FeedbackCoverage struct {
	Complete bool   `json:"complete"`
	Fetched  int    `json:"fetched"`
	Total    int    `json:"total"`
	Reason   string `json:"reason,omitempty"`
}

type FeedbackComment struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id,omitempty"`
	Author      string    `json:"author,omitempty"`
	Body        string    `json:"body,omitempty"`
	Path        string    `json:"path,omitempty"`
	Line        *int      `json:"line,omitempty"`
	StartLine   *int      `json:"start_line,omitempty"`
	CommitOID   string    `json:"commit_oid,omitempty"`
	InReplyToID int64     `json:"in_reply_to_id,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Outdated    bool      `json:"outdated,omitempty"`
}

type FeedbackReview struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id,omitempty"`
	Author      string    `json:"author,omitempty"`
	State       string    `json:"state,omitempty"`
	Body        string    `json:"body,omitempty"`
	CommitOID   string    `json:"commit_oid,omitempty"`
	SubmittedAt time.Time `json:"submitted_at,omitempty"`
}

type FeedbackThread struct {
	ID         string            `json:"id"`
	Resolved   bool              `json:"resolved"`
	Outdated   bool              `json:"outdated"`
	Path       string            `json:"path,omitempty"`
	Line       *int              `json:"line,omitempty"`
	StartLine  *int              `json:"start_line,omitempty"`
	Comments   []FeedbackComment `json:"comments"`
	Truncated  bool              `json:"truncated"`
	TotalCount int               `json:"total_count"`
}

type PullRequestFeedback struct {
	HeadSHA         string                      `json:"head_sha"`
	SourceUpdatedAt time.Time                   `json:"source_updated_at"`
	IssueComments   []FeedbackComment           `json:"issue_comments,omitempty"`
	Reviews         []FeedbackReview            `json:"submitted_reviews,omitempty"`
	InlineComments  []FeedbackComment           `json:"inline_comments,omitempty"`
	ReviewThreads   []FeedbackThread            `json:"review_threads,omitempty"`
	Coverage        map[string]FeedbackCoverage `json:"coverage"`
}

type PullRequestFeedbackReader interface {
	GetPullRequestFeedback(context.Context, string, string, int, PullRequestFeedbackOptions, *RequestBudget) (PullRequestFeedback, error)
}

func (c *Client) GetPullRequestFeedback(ctx context.Context, owner, repo string, number int, opts PullRequestFeedbackOptions, budget *RequestBudget) (PullRequestFeedback, error) {
	out := PullRequestFeedback{Coverage: make(map[string]FeedbackCoverage)}
	if err := budget.Take(); err != nil {
		return out, err
	}
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return out, classifyError(err)
	}
	out.HeadSHA = pr.GetHead().GetSHA()
	out.SourceUpdatedAt = pr.GetUpdatedAt().Time
	for _, channel := range opts.Channels {
		var err error
		switch channel {
		case "issue_comments":
			out.IssueComments, out.Coverage[channel], err = c.feedbackIssueComments(ctx, owner, repo, number, opts.MaxItemsPerChannel, budget)
		case "submitted_reviews":
			out.Reviews, out.Coverage[channel], err = c.feedbackReviews(ctx, owner, repo, number, opts.MaxItemsPerChannel, budget)
		case "inline_comments":
			out.InlineComments, out.Coverage[channel], err = c.feedbackInlineComments(ctx, owner, repo, number, opts.MaxItemsPerChannel, budget)
		case "review_threads":
			var updated time.Time
			var graphHead string
			out.ReviewThreads, graphHead, updated, out.Coverage[channel], err = c.feedbackReviewThreads(ctx, owner, repo, number, opts, budget)
			if err == nil && graphHead != out.HeadSHA {
				err = &TransientError{Cause: errors.New("pull request head changed while feedback was fetched")}
			}
			if updated.After(out.SourceUpdatedAt) {
				out.SourceUpdatedAt = updated
			}
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *Client) feedbackIssueComments(ctx context.Context, owner, repo string, number, limit int, budget *RequestBudget) ([]FeedbackComment, FeedbackCoverage, error) {
	items := make([]FeedbackComment, 0, limit)
	page := 1
	for len(items) < limit {
		if err := budget.Take(); err != nil {
			return items, FeedbackCoverage{Fetched: len(items), Reason: "request_budget_exhausted"}, err
		}
		perPage := min(100, limit-len(items))
		values, resp, err := c.gh.Issues.ListComments(ctx, owner, repo, number, &gh.IssueListCommentsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}})
		if err != nil {
			return items, FeedbackCoverage{Fetched: len(items)}, classifyError(err)
		}
		for _, value := range values {
			items = append(items, FeedbackComment{ID: value.GetID(), NodeID: value.GetNodeID(), Author: value.GetUser().GetLogin(), Body: value.GetBody(), CreatedAt: value.GetCreatedAt().Time, UpdatedAt: value.GetUpdatedAt().Time})
		}
		if resp == nil || resp.NextPage == 0 {
			return items, FeedbackCoverage{Complete: true, Fetched: len(items), Total: len(items)}, nil
		}
		page = resp.NextPage
	}
	return items, FeedbackCoverage{Fetched: len(items), Total: len(items) + 1, Reason: "item_limit_reached"}, nil
}

func (c *Client) feedbackReviews(ctx context.Context, owner, repo string, number, limit int, budget *RequestBudget) ([]FeedbackReview, FeedbackCoverage, error) {
	items := make([]FeedbackReview, 0, limit)
	page := 1
	for len(items) < limit {
		if err := budget.Take(); err != nil {
			return items, FeedbackCoverage{Fetched: len(items), Reason: "request_budget_exhausted"}, err
		}
		values, resp, err := c.gh.PullRequests.ListReviews(ctx, owner, repo, number, &gh.ListOptions{Page: page, PerPage: min(100, limit-len(items))})
		if err != nil {
			return items, FeedbackCoverage{Fetched: len(items)}, classifyError(err)
		}
		for _, value := range values {
			items = append(items, FeedbackReview{ID: value.GetID(), NodeID: value.GetNodeID(), Author: value.GetUser().GetLogin(), State: value.GetState(), Body: value.GetBody(), CommitOID: value.GetCommitID(), SubmittedAt: value.GetSubmittedAt().Time})
		}
		if resp == nil || resp.NextPage == 0 {
			return items, FeedbackCoverage{Complete: true, Fetched: len(items), Total: len(items)}, nil
		}
		page = resp.NextPage
	}
	return items, FeedbackCoverage{Fetched: len(items), Total: len(items) + 1, Reason: "item_limit_reached"}, nil
}

func (c *Client) feedbackInlineComments(ctx context.Context, owner, repo string, number, limit int, budget *RequestBudget) ([]FeedbackComment, FeedbackCoverage, error) {
	items := make([]FeedbackComment, 0, limit)
	page := 1
	for len(items) < limit {
		if err := budget.Take(); err != nil {
			return items, FeedbackCoverage{Fetched: len(items), Reason: "request_budget_exhausted"}, err
		}
		values, resp, err := c.gh.PullRequests.ListComments(ctx, owner, repo, number, &gh.PullRequestListCommentsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: min(100, limit-len(items))}})
		if err != nil {
			return items, FeedbackCoverage{Fetched: len(items)}, classifyError(err)
		}
		for _, value := range values {
			items = append(items, FeedbackComment{
				ID: value.GetID(), NodeID: value.GetNodeID(), Author: value.GetUser().GetLogin(), Body: value.GetBody(),
				Path: value.GetPath(), Line: value.Line, StartLine: value.StartLine, CommitOID: value.GetCommitID(),
				InReplyToID: value.GetInReplyTo(), CreatedAt: value.GetCreatedAt().Time, UpdatedAt: value.GetUpdatedAt().Time,
				Outdated: value.GetPosition() == 0 && value.Position != nil,
			})
		}
		if resp == nil || resp.NextPage == 0 {
			return items, FeedbackCoverage{Complete: true, Fetched: len(items), Total: len(items)}, nil
		}
		page = resp.NextPage
	}
	return items, FeedbackCoverage{Fetched: len(items), Total: len(items) + 1, Reason: "item_limit_reached"}, nil
}

const pullRequestFeedbackThreadsQuery = `query PullRequestFeedback($owner: String!, $repo: String!, $number: Int!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      headRefOid
      updatedAt
      reviewThreads(first: $first, after: $after) {
        totalCount
        nodes {
          id isResolved isOutdated path line startLine
          comments(first: 100) {
            totalCount
            nodes { id databaseId body createdAt updatedAt path line startLine outdated commit { oid } author { login } replyTo { databaseId } }
            pageInfo { hasNextPage endCursor }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type feedbackThreadsEnvelope struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				HeadSHA   string                   `json:"headRefOid"`
				UpdatedAt time.Time                `json:"updatedAt"`
				Threads   feedbackThreadConnection `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors"`
}

type feedbackThreadConnection struct {
	TotalCount int `json:"totalCount"`
	Nodes      []struct {
		ID, Path               string
		IsResolved, IsOutdated bool
		Line, StartLine        *int
		Comments               feedbackCommentConnection
	} `json:"nodes"`
	PageInfo graphQLPageInfo `json:"pageInfo"`
}

type feedbackCommentConnection struct {
	TotalCount int `json:"totalCount"`
	Nodes      []feedbackCommentNode
	PageInfo   graphQLPageInfo `json:"pageInfo"`
}

type feedbackCommentNode struct {
	ID                   string
	DatabaseID           int64 `json:"databaseId"`
	Body, Path           string
	CreatedAt, UpdatedAt time.Time
	Line, StartLine      *int
	Outdated             bool
	Commit               *struct {
		OID string `json:"oid"`
	}
	Author *struct {
		Login string `json:"login"`
	}
	ReplyTo *struct {
		DatabaseID int64 `json:"databaseId"`
	}
}

const reviewThreadCommentsQuery = `query PullRequestReviewThreadComments($id: ID!, $first: Int!, $after: String) {
  node(id: $id) {
    ... on PullRequestReviewThread {
      comments(first: $first, after: $after) {
        totalCount
        nodes { id databaseId body createdAt updatedAt path line startLine outdated commit { oid } author { login } replyTo { databaseId } }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

func (c *Client) feedbackReviewThreads(ctx context.Context, owner, repo string, number int, opts PullRequestFeedbackOptions, budget *RequestBudget) ([]FeedbackThread, string, time.Time, FeedbackCoverage, error) {
	items := make([]FeedbackThread, 0, opts.MaxItemsPerChannel)
	cursor := ""
	total := 0
	var head string
	var updated time.Time
	for len(items) < opts.MaxItemsPerChannel {
		if err := budget.Take(); err != nil {
			return items, head, updated, FeedbackCoverage{Fetched: len(items), Total: total, Reason: "request_budget_exhausted"}, err
		}
		body := graphQLRequest{Query: pullRequestFeedbackThreadsQuery, Variables: map[string]any{
			"owner": owner, "repo": repo, "number": number, "first": min(100, opts.MaxItemsPerChannel-len(items)), "after": optionalGraphQLCursor(cursor),
		}}
		req, err := c.gh.NewRequest(ctx, http.MethodPost, "graphql", body)
		if err != nil {
			return nil, "", time.Time{}, FeedbackCoverage{}, err
		}
		req = markReplayableRead(req)
		var envelope feedbackThreadsEnvelope
		if _, err := c.gh.Do(req, &envelope); err != nil {
			return nil, "", time.Time{}, FeedbackCoverage{}, classifyError(err)
		}
		if len(envelope.Errors) > 0 {
			return nil, "", time.Time{}, FeedbackCoverage{}, fmt.Errorf("github graphql: %s", envelope.Errors[0].Message)
		}
		pr := envelope.Data.Repository.PullRequest
		if head == "" {
			head, updated = pr.HeadSHA, pr.UpdatedAt
		} else if head != pr.HeadSHA || !updated.Equal(pr.UpdatedAt) {
			return nil, "", time.Time{}, FeedbackCoverage{}, &TransientError{Cause: errors.New("pull request changed while feedback was paged")}
		}
		total = pr.Threads.TotalCount
		for _, node := range pr.Threads.Nodes {
			if opts.ThreadState == "unresolved" && node.IsResolved {
				continue
			}
			thread := FeedbackThread{ID: node.ID, Resolved: node.IsResolved, Outdated: node.IsOutdated, Path: node.Path, Line: node.Line, StartLine: node.StartLine, TotalCount: node.Comments.TotalCount}
			thread.Comments = appendFeedbackComments(thread.Comments, node.Comments.Nodes)
			if node.Comments.PageInfo.HasNextPage {
				comments, err := c.pageReviewThreadComments(ctx, node.ID, node.Comments.PageInfo.EndCursor, budget)
				if err != nil {
					return nil, "", time.Time{}, FeedbackCoverage{}, err
				}
				thread.Comments = append(thread.Comments, comments...)
			}
			thread.Truncated = len(thread.Comments) < thread.TotalCount
			items = append(items, thread)
			if len(items) == opts.MaxItemsPerChannel {
				break
			}
		}
		if !pr.Threads.PageInfo.HasNextPage {
			return items, head, updated, FeedbackCoverage{Complete: true, Fetched: len(items), Total: total}, nil
		}
		cursor = pr.Threads.PageInfo.EndCursor
	}
	return items, head, updated, FeedbackCoverage{Fetched: len(items), Total: total, Reason: "item_limit_reached"}, nil
}

func appendFeedbackComments(dst []FeedbackComment, comments []feedbackCommentNode) []FeedbackComment {
	for _, comment := range comments {
		value := FeedbackComment{ID: comment.DatabaseID, NodeID: comment.ID, Body: comment.Body, Path: comment.Path, Line: comment.Line, StartLine: comment.StartLine, CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt, Outdated: comment.Outdated}
		if comment.Author != nil {
			value.Author = comment.Author.Login
		}
		if comment.Commit != nil {
			value.CommitOID = comment.Commit.OID
		}
		if comment.ReplyTo != nil {
			value.InReplyToID = comment.ReplyTo.DatabaseID
		}
		dst = append(dst, value)
	}
	return dst
}

func (c *Client) pageReviewThreadComments(ctx context.Context, id, cursor string, budget *RequestBudget) ([]FeedbackComment, error) {
	var items []FeedbackComment
	for {
		if err := budget.Take(); err != nil {
			return nil, err
		}
		body := graphQLRequest{Query: reviewThreadCommentsQuery, Variables: map[string]any{"id": id, "first": 100, "after": optionalGraphQLCursor(cursor)}}
		req, err := c.gh.NewRequest(ctx, http.MethodPost, "graphql", body)
		if err != nil {
			return nil, err
		}
		req = markReplayableRead(req)
		var envelope struct {
			Data struct {
				Node struct {
					Comments feedbackCommentConnection `json:"comments"`
				} `json:"node"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if _, err := c.gh.Do(req, &envelope); err != nil {
			return nil, classifyError(err)
		}
		if len(envelope.Errors) > 0 {
			return nil, fmt.Errorf("github graphql: %s", envelope.Errors[0].Message)
		}
		items = appendFeedbackComments(items, envelope.Data.Node.Comments.Nodes)
		if !envelope.Data.Node.Comments.PageInfo.HasNextPage {
			return items, nil
		}
		cursor = envelope.Data.Node.Comments.PageInfo.EndCursor
	}
}

type CIFailureOptions struct {
	MaxRuns       int
	MaxJobsPerRun int
	MaxLogBytes   int
	Logs          string
}

type CICheck struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"details_url,omitempty"`
}
type CIJobLog struct {
	JobID     int64  `json:"job_id"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated"`
}
type CIJob struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url,omitempty"`
	Steps      []string  `json:"steps,omitempty"`
	Log        *CIJobLog `json:"log,omitempty"`
}
type CIRun struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Event         string  `json:"event"`
	Status        string  `json:"status"`
	Conclusion    string  `json:"conclusion"`
	HTMLURL       string  `json:"html_url,omitempty"`
	Attempt       int     `json:"attempt"`
	Jobs          []CIJob `json:"jobs"`
	JobsTruncated bool    `json:"jobs_truncated"`
}
type PullRequestCI struct {
	HeadSHA   string                      `json:"head_sha"`
	Statuses  []CICheck                   `json:"statuses"`
	CheckRuns []CICheck                   `json:"check_runs"`
	Runs      []CIRun                     `json:"workflow_runs"`
	Coverage  map[string]FeedbackCoverage `json:"coverage"`
}
type PullRequestCIReader interface {
	GetPullRequestCI(context.Context, string, string, int, CIFailureOptions, *RequestBudget) (PullRequestCI, error)
}

func (c *Client) GetPullRequestCI(ctx context.Context, owner, repo string, number int, opts CIFailureOptions, budget *RequestBudget) (PullRequestCI, error) {
	out := PullRequestCI{Coverage: make(map[string]FeedbackCoverage)}
	if err := budget.Take(); err != nil {
		return out, err
	}
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return out, classifyError(err)
	}
	out.HeadSHA = pr.GetHead().GetSHA()

	for page := 1; ; {
		if err := budget.Take(); err != nil {
			return out, err
		}
		statuses, resp, err := c.gh.Repositories.GetCombinedStatus(ctx, owner, repo, out.HeadSHA, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return out, classifyError(err)
		}
		for _, status := range statuses.Statuses {
			out.Statuses = append(out.Statuses, CICheck{Kind: "status", Name: status.GetContext(), Status: status.GetState(), Conclusion: status.GetState(), DetailsURL: status.GetTargetURL()})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	out.Coverage["statuses"] = FeedbackCoverage{Complete: true, Fetched: len(out.Statuses), Total: len(out.Statuses)}

	checkTotal := 0
	for page := 1; ; {
		if err := budget.Take(); err != nil {
			return out, err
		}
		checks, resp, err := c.gh.Checks.ListCheckRunsForRef(ctx, owner, repo, out.HeadSHA, &gh.ListCheckRunsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return out, classifyError(err)
		}
		checkTotal = checks.GetTotal()
		for _, check := range checks.CheckRuns {
			out.CheckRuns = append(out.CheckRuns, CICheck{Kind: "check_run", Name: check.GetName(), Status: check.GetStatus(), Conclusion: check.GetConclusion(), DetailsURL: check.GetDetailsURL()})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	out.Coverage["check_runs"] = FeedbackCoverage{Complete: checkTotal <= len(out.CheckRuns), Fetched: len(out.CheckRuns), Total: checkTotal}

	if err := budget.Take(); err != nil {
		return out, err
	}
	runs, _, err := c.gh.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &gh.ListWorkflowRunsOptions{HeadSHA: out.HeadSHA, ListOptions: gh.ListOptions{PerPage: min(100, opts.MaxRuns)}})
	if err != nil {
		return out, classifyError(err)
	}
	for _, run := range runs.WorkflowRuns {
		if len(out.Runs) == opts.MaxRuns {
			break
		}
		item := CIRun{ID: run.GetID(), Name: run.GetName(), Event: run.GetEvent(), Status: run.GetStatus(), Conclusion: run.GetConclusion(), HTMLURL: run.GetHTMLURL(), Attempt: run.GetRunAttempt()}
		if err := budget.Take(); err != nil {
			return out, err
		}
		jobs, _, err := c.gh.Actions.ListWorkflowJobs(ctx, owner, repo, run.GetID(), &gh.ListWorkflowJobsOptions{Filter: "latest", ListOptions: gh.ListOptions{PerPage: min(100, opts.MaxJobsPerRun)}})
		if err != nil {
			return out, classifyError(err)
		}
		for _, job := range jobs.Jobs {
			if len(item.Jobs) == opts.MaxJobsPerRun {
				break
			}
			ciJob := CIJob{ID: job.GetID(), Name: job.GetName(), Status: job.GetStatus(), Conclusion: job.GetConclusion(), HTMLURL: job.GetHTMLURL()}
			for _, step := range job.Steps {
				ciJob.Steps = append(ciJob.Steps, step.GetName()+":"+step.GetConclusion())
			}
			if opts.Logs == "failures_only" && (job.GetConclusion() == "failure" || job.GetConclusion() == "timed_out") {
				log, err := c.downloadWorkflowJobLog(ctx, owner, repo, job.GetID(), opts.MaxLogBytes, budget)
				if err != nil {
					return out, err
				}
				ciJob.Log = &log
			}
			item.Jobs = append(item.Jobs, ciJob)
		}
		item.JobsTruncated = jobs.GetTotalCount() > len(item.Jobs)
		out.Runs = append(out.Runs, item)
	}
	out.Coverage["workflow_runs"] = FeedbackCoverage{Complete: runs.GetTotalCount() <= len(out.Runs), Fetched: len(out.Runs), Total: runs.GetTotalCount()}
	return out, nil
}

func (c *Client) downloadWorkflowJobLog(ctx context.Context, owner, repo string, jobID int64, limit int, budget *RequestBudget) (CIJobLog, error) {
	if err := budget.Take(); err != nil {
		return CIJobLog{}, err
	}
	location, _, err := c.gh.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 0)
	if err != nil {
		return CIJobLog{}, classifyError(err)
	}
	if err := budget.Take(); err != nil {
		return CIJobLog{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return CIJobLog{}, err
	}
	if err := validateLogDownloadURL(location); err != nil {
		return CIJobLog{}, err
	}
	downloadClient := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := validateLogDownloadURL(req.URL); err != nil {
				return err
			}
			if len(via) >= 3 {
				return errors.New("too many CI log redirects")
			}
			return nil
		},
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return CIJobLog{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CIJobLog{}, fmt.Errorf("download workflow job log: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return CIJobLog{}, err
	}
	truncated := len(data) > limit
	if truncated {
		data = data[:limit]
	}
	return CIJobLog{JobID: jobID, Body: strings.ToValidUTF8(string(data), "�"), Truncated: truncated}, nil
}

func validateLogDownloadURL(location *url.URL) error {
	if location == nil || location.Host == "" || location.User != nil {
		return errors.New("invalid CI log download URL")
	}
	if location.Scheme == "https" {
		return nil
	}
	host := location.Hostname()
	if location.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return errors.New("CI log download URL must use HTTPS")
}
