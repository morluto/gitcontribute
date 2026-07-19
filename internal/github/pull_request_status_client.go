package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const pullRequestStatusQuery = `query PullRequestStatus($owner: String!, $repo: String!, $number: Int!, $first: Int!, $checksAfter: String, $threadsAfter: String, $issuesAfter: String, $filesAfter: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      updatedAt
      headRefOid
      mergeStateStatus
      mergeable
      mergeQueueEntry { id state position enqueuedAt estimatedTimeToMerge }
      closingIssuesReferences(first: $first, after: $issuesAfter) {
        totalCount
        nodes { id number url repository { nameWithOwner } }
        pageInfo { hasNextPage endCursor }
      }
      files(first: $first, after: $filesAfter) {
        totalCount
        nodes { path changeType additions deletions }
        pageInfo { hasNextPage endCursor }
      }
      reviewThreads(first: $first, after: $threadsAfter) {
        totalCount
        nodes { id isResolved isOutdated path line startLine }
        pageInfo { hasNextPage endCursor }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first: $first, after: $checksAfter) {
                totalCount
                nodes {
                  __typename
                  ... on CheckRun { name status conclusion detailsUrl startedAt completedAt }
                  ... on StatusContext { context state targetUrl createdAt }
                }
                pageInfo { hasNextPage endCursor }
              }
            }
          }
        }
      }
    }
  }
}`

const (
	defaultPullRequestStatusPageSize = 100
	maxPullRequestStatusPageSize     = 100
	defaultPullRequestStatusMaxPages = 10
)

// GetPullRequestStatus reads one bounded GraphQL snapshot of the PR's health
// facts. Collection truncation is returned as incomplete coverage; it is never
// silently treated as an empty or complete facet.
func (c *Client) GetPullRequestStatus(ctx context.Context, owner, name string, number int, opts PullRequestStatusOptions) (PullRequestStatus, error) {
	first := opts.PageSize
	if first <= 0 {
		first = defaultPullRequestStatusPageSize
	}
	if first > maxPullRequestStatusPageSize {
		first = maxPullRequestStatusPageSize
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = defaultPullRequestStatusMaxPages
	}
	var result PullRequestStatus
	var cursors pullRequestStatusCursors
	active := pullRequestStatusActive{checks: true, threads: true, issues: true, files: true}
	for page := 0; page < maxPages; page++ {
		pageResult, err := c.getPullRequestStatusPage(ctx, owner, name, number, first, cursors)
		if err != nil {
			return PullRequestStatus{}, err
		}
		if page == 0 {
			result = pageResult
		} else {
			if pageResult.NodeID != result.NodeID || pageResult.HeadSHA != result.HeadSHA || !pageResult.SourceUpdatedAt.Equal(result.SourceUpdatedAt) {
				return PullRequestStatus{}, &TransientError{Cause: errors.New("pull request changed while status facets were being paged")}
			}
			mergePullRequestStatusPage(&result, pageResult, active)
		}
		active = activePullRequestStatusFacets(result)
		if !active.any() {
			return result, nil
		}
		cursors = pullRequestStatusCursors{
			checks: result.Checks.Coverage.EndCursor, threads: result.ReviewThreads.Coverage.EndCursor,
			issues: result.ClosingIssues.Coverage.EndCursor, files: result.Files.Coverage.EndCursor,
		}
	}
	return result, nil
}

func (c *Client) getPullRequestStatusPage(ctx context.Context, owner, name string, number, first int, cursors pullRequestStatusCursors) (PullRequestStatus, error) {
	body := graphQLRequest{
		Query: pullRequestStatusQuery,
		Variables: map[string]any{
			"owner": owner, "repo": name, "number": number, "first": first,
			"checksAfter": optionalGraphQLCursor(cursors.checks), "threadsAfter": optionalGraphQLCursor(cursors.threads),
			"issuesAfter": optionalGraphQLCursor(cursors.issues), "filesAfter": optionalGraphQLCursor(cursors.files),
		},
	}
	req, err := c.gh.NewRequest(ctx, http.MethodPost, "graphql", body)
	if err != nil {
		return PullRequestStatus{}, fmt.Errorf("create pull request status request: %w", err)
	}
	var envelope pullRequestStatusEnvelope
	resp, err := c.gh.Do(req, &envelope)
	if err != nil {
		return PullRequestStatus{}, classifyError(err)
	}
	if len(envelope.Errors) != 0 {
		messages := make([]string, 0, len(envelope.Errors))
		rateLimited, transient := false, false
		for _, graphErr := range envelope.Errors {
			messages = append(messages, graphErr.Message)
			switch strings.ToUpper(graphErr.Type) {
			case "RATE_LIMITED":
				rateLimited = true
			case "INTERNAL", "SERVICE_UNAVAILABLE", "TIMEOUT":
				transient = true
			}
		}
		cause := fmt.Errorf("github graphql: %s", strings.Join(messages, "; "))
		if rateLimited {
			rate := RateInfo{}
			if resp != nil {
				rate = rateInfo(resp.Rate)
			}
			retryAfter := time.Second
			if !rate.Reset.IsZero() && time.Until(rate.Reset) > 0 {
				retryAfter = time.Until(rate.Reset)
			}
			return PullRequestStatus{}, &PrimaryRateLimitError{Rate: rate, RetryAfter: retryAfter, Message: cause.Error()}
		}
		if transient {
			return PullRequestStatus{}, &TransientError{Cause: cause}
		}
		return PullRequestStatus{}, cause
	}
	if envelope.Data.Repository == nil || envelope.Data.Repository.PullRequest == nil {
		return PullRequestStatus{}, &NotFoundError{Resource: fmt.Sprintf("pull request %s/%s#%d", owner, name, number)}
	}
	return convertPullRequestStatus(*envelope.Data.Repository.PullRequest), nil
}
