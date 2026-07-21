package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v89/github"
)

const (
	DefaultBaseURL           = "https://api.github.com/"
	DefaultUploadURL         = "https://uploads.github.com/"
	DefaultRequestsPerSecond = 10.0
	DefaultBurst             = 20
)

// Reader is the product-owned read contract for GitHub.
type Reader interface {
	GetRepository(ctx context.Context, owner, name string) (Repository, RateInfo, error)
	ListIssues(ctx context.Context, owner, name string, opts ListIssueOptions) (ListResult[Issue], error)
	ListIssueComments(ctx context.Context, owner, name string, issueNumber int, opts PageOptions) (ListResult[IssueComment], error)
	GetPullRequestDetails(ctx context.Context, owner, name string, number int) (PullRequestDetails, RateInfo, error)
	ListPullRequestReviews(ctx context.Context, owner, name string, number int, opts PageOptions) (ListResult[Review], error)
	ListPullRequestComments(ctx context.Context, owner, name string, number int, opts PageOptions) (ListResult[ReviewComment], error)
}

// IssueGetter is the optional exact-thread capability used by bounded archive
// refreshes. Keeping it separate avoids forcing broad discovery readers to
// implement an operation they do not need.
type IssueGetter interface {
	GetIssue(ctx context.Context, owner, name string, number int) (Issue, RateInfo, error)
}

// IssueTimelineReader is the optional, paginated issue-history capability.
// It stays separate from Reader so archive-only and test readers remain narrow.
type IssueTimelineReader interface {
	ListIssueTimeline(context.Context, string, string, int, PageOptions) (ListResult[IssueTimelineEvent], error)
}

// RepositoryFileReader is the optional exact-file capability used to ingest a
// small, fixed set of contribution-policy documents during explicit syncs.
type RepositoryFileReader interface {
	GetRepositoryFile(ctx context.Context, owner, name, path string) (RepositoryFile, RateInfo, error)
}

// RepositorySearcher is the optional GitHub Search capability used by broad
// discovery. Keeping it separate lets archive-only readers stay small.
type RepositorySearcher interface {
	SearchRepositories(ctx context.Context, opts RepositorySearchOptions) (RepositorySearchResult, error)
}

// IdentityReader resolves the authenticated GitHub account without granting
// any mutation capability.
type IdentityReader interface {
	GetAuthenticatedIdentity(context.Context) (Identity, RateInfo, error)
}

// AuthoredPullRequestSearcher discovers pull requests authored by one login.
type AuthoredPullRequestSearcher interface {
	SearchAuthoredPullRequests(context.Context, AuthoredPullRequestSearchOptions) (AuthoredPullRequestSearchResult, error)
}

// PullRequestStatusReader reads bounded, source-backed PR health facets.
type PullRequestStatusReader interface {
	GetPullRequestStatus(context.Context, string, string, int, PullRequestStatusOptions) (PullRequestStatus, error)
}

// Client wraps go-github behind a narrow, domain-neutral interface.
type Client struct {
	gh *gh.Client
}

// Config controls how the GitHub client is constructed.
type Config struct {
	BaseURL           string
	UploadURL         string
	TokenSource       TokenSource
	HTTPClient        *http.Client
	Limiter           Limiter
	RequestsPerSecond float64
	Burst             int
	Retry             *RetryConfig
}

// NewClient creates a GitHub read client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.UploadURL == "" {
		cfg.UploadURL = DefaultUploadURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}

	baseTransport := cfg.HTTPClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	limiter := cfg.Limiter
	if limiter == nil {
		rps := cfg.RequestsPerSecond
		if rps <= 0 {
			rps = DefaultRequestsPerSecond
		}
		burst := cfg.Burst
		if burst <= 0 {
			burst = DefaultBurst
		}
		limiter = NewRateLimiter(rps, burst)
	}

	retryCfg := cfg.Retry
	if retryCfg == nil {
		retryCfg = DefaultRetryConfig()
	}
	retryCfg = retryCfg.withDefaults()

	retrier := &retryTransport{
		Base:   &RateLimitedTransport{Base: baseTransport, Limiter: limiter},
		Config: retryCfg,
		cb:     newCircuitBreaker(defaultCBMaxFailures, defaultCBHalfOpenWait, defaultCBProbeTimeout),
	}

	cfg.HTTPClient.Transport = &authTransport{
		Base:   retrier,
		Source: cfg.TokenSource,
	}

	opts := []gh.ClientOptionsFunc{
		gh.WithHTTPClient(cfg.HTTPClient),
		gh.WithEnterpriseURLs(cfg.BaseURL, cfg.UploadURL),
	}

	ghc, err := gh.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{gh: ghc}, nil
}

// GetRepository reads repository metadata and the response rate-limit state.
func (c *Client) GetRepository(ctx context.Context, owner, name string) (Repository, RateInfo, error) {
	repo, resp, err := c.gh.Repositories.Get(ctx, owner, name)
	if err != nil {
		return Repository{}, RateInfo{}, classifyError(err)
	}
	return convertRepository(repo), rateInfo(resp.Rate), nil
}

// GetRepositoryFile reads one text file from the repository's default branch.
func (c *Client) GetRepositoryFile(ctx context.Context, owner, name, path string) (RepositoryFile, RateInfo, error) {
	file, _, resp, err := c.gh.Repositories.GetContents(ctx, owner, name, path, nil)
	if err != nil {
		return RepositoryFile{}, responseRateInfo(resp), classifyError(err)
	}
	if file == nil {
		return RepositoryFile{}, responseRateInfo(resp), &NotFoundError{Resource: path}
	}
	content, err := file.GetContent()
	if err != nil {
		return RepositoryFile{}, responseRateInfo(resp), fmt.Errorf("decode repository file %q: %w", path, err)
	}
	return RepositoryFile{
		Path: file.GetPath(), SHA: file.GetSHA(), HTMLURL: file.GetHTMLURL(), Content: content,
	}, responseRateInfo(resp), nil
}

// ListIssueTimeline reads one REST timeline page without exposing go-github
// models beyond the adapter boundary.
func (c *Client) ListIssueTimeline(ctx context.Context, owner, name string, number int, opts PageOptions) (ListResult[IssueTimelineEvent], error) {
	items, resp, err := c.gh.Issues.ListIssueTimeline(ctx, owner, name, number, &gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage})
	if err != nil {
		return ListResult[IssueTimelineEvent]{}, classifyError(err)
	}
	converted := make([]IssueTimelineEvent, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		event := IssueTimelineEvent{ID: item.GetID(), Event: item.GetEvent(), CommitID: item.GetCommitID(), CreatedAt: item.GetCreatedAt().Time}
		if actor := item.GetActor(); actor != nil {
			event.Actor = actor.GetLogin()
		}
		if source := item.GetSource(); source != nil {
			if issue := source.GetIssue(); issue != nil {
				event.SourceNumber = issue.GetNumber()
				event.SourceIsPullRequest = issue.IsPullRequest()
				if repository := issue.GetRepository(); repository != nil {
					event.SourceRepository = repository.GetName()
					if repository.Owner != nil {
						event.SourceOwner = repository.Owner.GetLogin()
					}
				}
			}
		}
		converted = append(converted, event)
	}
	return ListResult[IssueTimelineEvent]{Items: converted, Page: pageInfo(resp), Rate: rateInfo(resp.Rate)}, nil
}

// GetAuthenticatedIdentity resolves the user associated with the configured
// read credential.
func (c *Client) GetAuthenticatedIdentity(ctx context.Context) (Identity, RateInfo, error) {
	user, resp, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return Identity{}, RateInfo{}, classifyError(err)
	}
	return Identity{Login: user.GetLogin(), ID: user.GetID(), NodeID: user.GetNodeID()}, rateInfo(resp.Rate), nil
}

// SearchAuthoredPullRequests searches one bounded page of PRs authored by a
// login and preserves GitHub's incomplete-results signal.
func (c *Client) SearchAuthoredPullRequests(ctx context.Context, opts AuthoredPullRequestSearchOptions) (AuthoredPullRequestSearchResult, error) {
	query := "is:pr author:" + opts.Login
	if opts.State != "" && opts.State != "all" {
		query += " is:" + opts.State
	}
	if !opts.UpdatedAfter.IsZero() {
		query += " updated:>=" + opts.UpdatedAfter.UTC().Format("2006-01-02")
	}
	result, resp, err := c.gh.Search.Issues(ctx, query, &gh.SearchOptions{Sort: "updated", Order: "desc", ListOptions: gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage}})
	if err != nil {
		return AuthoredPullRequestSearchResult{}, classifyError(err)
	}
	items := make([]Issue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		items = append(items, convertIssue(issue))
	}
	return AuthoredPullRequestSearchResult{Total: result.GetTotal(), Incomplete: result.GetIncompleteResults(), Items: items, Page: pageInfo(resp), Rate: rateInfo(resp.Rate)}, nil
}

// SearchRepositories reads one page from GitHub's repository Search API.
func (c *Client) SearchRepositories(ctx context.Context, opts RepositorySearchOptions) (RepositorySearchResult, error) {
	result, resp, err := c.gh.Search.Repositories(ctx, opts.Query, &gh.SearchOptions{
		Sort:        opts.Sort,
		Order:       opts.Order,
		ListOptions: gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	})
	if err != nil {
		return RepositorySearchResult{}, classifyError(err)
	}
	items := make([]Repository, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		items = append(items, convertRepository(repo))
	}
	return RepositorySearchResult{
		Total: result.GetTotal(), Incomplete: result.GetIncompleteResults(), Items: items,
		Page: pageInfo(resp), Rate: rateInfo(resp.Rate),
	}, nil
}

// ListIssues reads one page of issues and pull-request markers for a repository.
func (c *Client) ListIssues(ctx context.Context, owner, name string, opts ListIssueOptions) (ListResult[Issue], error) {
	gopts := &gh.IssueListByRepoOptions{
		State:     opts.State,
		Sort:      opts.Sort,
		Direction: opts.Direction,
		Labels:    opts.Labels,
		Since:     opts.Since,
	}
	gopts.ListOptions = gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage}

	issues, resp, err := c.gh.Issues.ListByRepo(ctx, owner, name, gopts)
	if err != nil {
		return ListResult[Issue]{}, classifyError(err)
	}

	items := make([]Issue, len(issues))
	for i, issue := range issues {
		items[i] = convertIssue(issue)
	}
	return ListResult[Issue]{
		Items: items,
		Page:  pageInfo(resp),
		Rate:  rateInfo(resp.Rate),
	}, nil
}

// GetIssue reads one issue or pull-request marker by number.
func (c *Client) GetIssue(ctx context.Context, owner, name string, number int) (Issue, RateInfo, error) {
	issue, resp, err := c.gh.Issues.Get(ctx, owner, name, number)
	if err != nil {
		return Issue{}, RateInfo{}, classifyError(err)
	}
	return convertIssue(issue), rateInfo(resp.Rate), nil
}

// ListIssueComments reads one page of issue comments for a thread.
func (c *Client) ListIssueComments(ctx context.Context, owner, name string, issueNumber int, opts PageOptions) (ListResult[IssueComment], error) {
	gopts := &gh.IssueListCommentsOptions{}
	gopts.ListOptions = gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage}

	comments, resp, err := c.gh.Issues.ListComments(ctx, owner, name, issueNumber, gopts)
	if err != nil {
		return ListResult[IssueComment]{}, classifyError(err)
	}

	items := make([]IssueComment, len(comments))
	for i, comment := range comments {
		items[i] = convertIssueComment(comment)
	}
	return ListResult[IssueComment]{
		Items: items,
		Page:  pageInfo(resp),
		Rate:  rateInfo(resp.Rate),
	}, nil
}

// GetPullRequestDetails reads pull-request metadata not present on issue list rows.
func (c *Client) GetPullRequestDetails(ctx context.Context, owner, name string, number int) (PullRequestDetails, RateInfo, error) {
	pr, resp, err := c.gh.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return PullRequestDetails{}, RateInfo{}, classifyError(err)
	}
	return convertPullRequestDetails(pr), rateInfo(resp.Rate), nil
}

// ListPullRequestReviews reads one page of pull-request reviews.
func (c *Client) ListPullRequestReviews(ctx context.Context, owner, name string, number int, opts PageOptions) (ListResult[Review], error) {
	reviews, resp, err := c.gh.PullRequests.ListReviews(ctx, owner, name, number, &gh.ListOptions{
		Page:    opts.Page,
		PerPage: opts.PerPage,
	})
	if err != nil {
		return ListResult[Review]{}, classifyError(err)
	}

	items := make([]Review, len(reviews))
	for i, review := range reviews {
		items[i] = convertReview(review)
	}
	return ListResult[Review]{
		Items: items,
		Page:  pageInfo(resp),
		Rate:  rateInfo(resp.Rate),
	}, nil
}

// ListPullRequestComments reads one page of pull-request review comments.
func (c *Client) ListPullRequestComments(ctx context.Context, owner, name string, number int, opts PageOptions) (ListResult[ReviewComment], error) {
	gopts := &gh.PullRequestListCommentsOptions{}
	gopts.ListOptions = gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage}

	comments, resp, err := c.gh.PullRequests.ListComments(ctx, owner, name, number, gopts)
	if err != nil {
		return ListResult[ReviewComment]{}, classifyError(err)
	}

	items := make([]ReviewComment, len(comments))
	for i, comment := range comments {
		items[i] = convertReviewComment(comment)
	}
	return ListResult[ReviewComment]{
		Items: items,
		Page:  pageInfo(resp),
		Rate:  rateInfo(resp.Rate),
	}, nil
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var rl *gh.RateLimitError
	if errors.As(err, &rl) {
		return &PrimaryRateLimitError{
			Rate:       rateInfo(rl.Rate),
			RetryAfter: time.Until(rl.Rate.Reset.Time),
			Message:    rl.Message,
		}
	}

	var abuse *gh.AbuseRateLimitError
	if errors.As(err, &abuse) {
		ra := time.Duration(0)
		if abuse.RetryAfter != nil {
			ra = *abuse.RetryAfter
		}
		return &SecondaryRateLimitError{
			RetryAfter: ra,
			Message:    abuse.Message,
		}
	}

	var er *gh.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		switch {
		case er.Response.StatusCode == http.StatusNotFound:
			return &NotFoundError{Resource: er.Message}
		case er.Response.StatusCode == http.StatusUnauthorized || er.Response.StatusCode == http.StatusForbidden:
			return &AccessDeniedError{StatusCode: er.Response.StatusCode, Message: er.Message}
		case er.Response.StatusCode == http.StatusGone:
			return &GoneError{Resource: er.Message}
		case er.Response.StatusCode >= 500:
			return &TransientError{Cause: err}
		}
	}

	return err
}

func rateInfo(r gh.Rate) RateInfo {
	return RateInfo{
		Limit:     r.Limit,
		Remaining: r.Remaining,
		Used:      r.Used,
		Reset:     r.Reset.Time,
		Resource:  r.Resource,
	}
}

func responseRateInfo(resp *gh.Response) RateInfo {
	if resp == nil {
		return RateInfo{}
	}
	return rateInfo(resp.Rate)
}

func pageInfo(resp *gh.Response) PageInfo {
	if resp == nil {
		return PageInfo{}
	}
	p := PageInfo{
		NextPage:  resp.NextPage,
		PrevPage:  resp.PrevPage,
		FirstPage: resp.FirstPage,
		LastPage:  resp.LastPage,
		HasNext:   resp.NextPage != 0,
		HasPrev:   resp.PrevPage != 0,
		HasFirst:  resp.FirstPage != 0,
		HasLast:   resp.LastPage != 0,
	}
	if resp.Response != nil && resp.Request != nil && resp.Request.URL != nil {
		q := resp.Request.URL.Query()
		if v := q.Get("page"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				p.Page = n
			}
		}
		if v := q.Get("per_page"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				p.PerPage = n
			}
		}
	}
	if p.PerPage == 0 {
		p.PerPage = 30
	}
	return p
}

func stringVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func intVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func int64Val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func boolVal(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func timeVal(t *gh.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

func timePtr(t *gh.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.Time
	return &v
}

func labelNames(labels []*gh.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if l == nil {
			continue
		}
		if n := l.GetName(); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func userLogins(users []*gh.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		if n := u.GetLogin(); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func convertRepository(r *gh.Repository) Repository {
	if r == nil {
		return Repository{}
	}
	openIssues := r.GetOpenIssuesCount()
	if openIssues == 0 {
		openIssues = r.GetOpenIssues()
	}
	return Repository{
		ID:            r.GetID(),
		NodeID:        r.GetNodeID(),
		Owner:         r.Owner.GetLogin(),
		Name:          r.GetName(),
		FullName:      r.GetFullName(),
		Description:   r.GetDescription(),
		DefaultBranch: r.GetDefaultBranch(),
		HTMLURL:       r.GetHTMLURL(),
		Private:       r.GetPrivate(),
		Fork:          r.GetFork(),
		Archived:      r.GetArchived(),
		IsTemplate:    r.GetIsTemplate(),
		Stars:         r.GetStargazersCount(),
		Watchers:      r.GetWatchersCount(),
		Forks:         r.GetForksCount(),
		OpenIssues:    openIssues,
		Language:      r.GetLanguage(),
		License:       r.License.GetName(),
		Topics:        r.Topics,
		CreatedAt:     timeVal(r.CreatedAt),
		UpdatedAt:     timeVal(r.UpdatedAt),
		PushedAt:      timePtr(r.PushedAt),
	}
}

func convertIssue(i *gh.Issue) Issue {
	if i == nil {
		return Issue{}
	}
	kind := ThreadKindIssue
	prURL := ""
	if i.PullRequestLinks != nil {
		kind = ThreadKindPullRequest
		prURL = i.PullRequestLinks.GetHTMLURL()
	}
	owner, repo := repositoryFromAPIURL(i.GetRepositoryURL())
	return Issue{
		RepositoryOwner:   owner,
		RepositoryName:    repo,
		ID:                i.GetID(),
		NodeID:            i.GetNodeID(),
		Number:            i.GetNumber(),
		Kind:              kind,
		Title:             i.GetTitle(),
		Body:              i.GetBody(),
		State:             i.GetState(),
		StateReason:       i.GetStateReason(),
		Draft:             i.GetDraft(),
		Locked:            i.GetLocked(),
		Author:            i.User.GetLogin(),
		AuthorAssociation: i.GetAuthorAssociation(),
		Labels:            labelNames(i.Labels),
		Assignees:         userLogins(i.Assignees),
		Milestone:         i.Milestone.GetTitle(),
		CommentsCount:     i.GetComments(),
		CreatedAt:         timeVal(i.CreatedAt),
		UpdatedAt:         timeVal(i.UpdatedAt),
		ClosedAt:          timePtr(i.ClosedAt),
		HTMLURL:           i.GetHTMLURL(),
		PullRequestURL:    prURL,
	}
}

func repositoryFromAPIURL(raw string) (string, string) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(raw), "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func convertIssueComment(c *gh.IssueComment) IssueComment {
	if c == nil {
		return IssueComment{}
	}
	return IssueComment{
		ID:                c.GetID(),
		NodeID:            c.GetNodeID(),
		Body:              c.GetBody(),
		Author:            c.User.GetLogin(),
		AuthorAssociation: c.GetAuthorAssociation(),
		CreatedAt:         timeVal(c.CreatedAt),
		UpdatedAt:         timeVal(c.UpdatedAt),
		HTMLURL:           c.GetHTMLURL(),
		IssueURL:          c.GetIssueURL(),
	}
}

func convertPullRequestDetails(pr *gh.PullRequest) PullRequestDetails {
	if pr == nil {
		return PullRequestDetails{}
	}
	headRef, headSHA := "", ""
	if pr.Head != nil {
		headRef = pr.Head.GetRef()
		headSHA = pr.Head.GetSHA()
	}
	baseRef, baseSHA := "", ""
	if pr.Base != nil {
		baseRef = pr.Base.GetRef()
		baseSHA = pr.Base.GetSHA()
	}
	return PullRequestDetails{
		ID:                pr.GetID(),
		NodeID:            pr.GetNodeID(),
		Number:            pr.GetNumber(),
		State:             pr.GetState(),
		Title:             pr.GetTitle(),
		Body:              pr.GetBody(),
		Draft:             pr.GetDraft(),
		Locked:            pr.GetLocked(),
		Author:            pr.User.GetLogin(),
		AuthorAssociation: pr.GetAuthorAssociation(),
		Labels:            labelNames(pr.Labels),
		Assignees:         userLogins(pr.Assignees),
		Milestone:         pr.Milestone.GetTitle(),
		CreatedAt:         timeVal(pr.CreatedAt),
		UpdatedAt:         timeVal(pr.UpdatedAt),
		ClosedAt:          timePtr(pr.ClosedAt),
		MergedAt:          timePtr(pr.MergedAt),
		Merged:            pr.GetMerged(),
		Mergeable:         pr.Mergeable,
		MergeCommitSHA:    pr.GetMergeCommitSHA(),
		HeadRef:           headRef,
		HeadSHA:           headSHA,
		BaseRef:           baseRef,
		BaseSHA:           baseSHA,
		CommentsCount:     pr.GetComments(),
		Commits:           pr.GetCommits(),
		Additions:         pr.GetAdditions(),
		Deletions:         pr.GetDeletions(),
		ChangedFiles:      pr.GetChangedFiles(),
		HTMLURL:           pr.GetHTMLURL(),
	}
}

func convertReview(r *gh.PullRequestReview) Review {
	if r == nil {
		return Review{}
	}
	return Review{
		ID:                r.GetID(),
		NodeID:            r.GetNodeID(),
		State:             r.GetState(),
		Body:              r.GetBody(),
		Author:            r.User.GetLogin(),
		AuthorAssociation: r.GetAuthorAssociation(),
		CommitID:          r.GetCommitID(),
		SubmittedAt:       timeVal(r.SubmittedAt),
		HTMLURL:           r.GetHTMLURL(),
		PullRequestURL:    r.GetPullRequestURL(),
	}
}

func convertReviewComment(c *gh.PullRequestComment) ReviewComment {
	if c == nil {
		return ReviewComment{}
	}
	return ReviewComment{
		ID:                c.GetID(),
		NodeID:            c.GetNodeID(),
		InReplyTo:         c.GetInReplyTo(),
		Body:              c.GetBody(),
		Path:              c.GetPath(),
		DiffHunk:          c.GetDiffHunk(),
		Author:            c.User.GetLogin(),
		AuthorAssociation: c.GetAuthorAssociation(),
		CommitID:          c.GetCommitID(),
		OriginalCommitID:  c.GetOriginalCommitID(),
		PullRequestURL:    c.GetPullRequestURL(),
		HTMLURL:           c.GetHTMLURL(),
		CreatedAt:         timeVal(c.CreatedAt),
		UpdatedAt:         timeVal(c.UpdatedAt),
		Line:              c.GetLine(),
		OriginalLine:      c.GetOriginalLine(),
		StartLine:         c.GetStartLine(),
		OriginalStartLine: c.GetOriginalStartLine(),
		Side:              c.GetSide(),
		StartSide:         c.GetStartSide(),
		Position:          c.GetPosition(),
		OriginalPosition:  c.GetOriginalPosition(),
		SubjectType:       c.GetSubjectType(),
	}
}
