package github

import (
	"strings"
	"time"
)

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type pullRequestStatusCursors struct {
	checks  string
	threads string
	issues  string
	files   string
}

type pullRequestStatusActive struct {
	checks  bool
	threads bool
	issues  bool
	files   bool
}

func (a pullRequestStatusActive) any() bool {
	return a.checks || a.threads || a.issues || a.files
}

func optionalGraphQLCursor(cursor string) any {
	if cursor == "" {
		return nil
	}
	return cursor
}

type pullRequestStatusEnvelope struct {
	Data struct {
		Repository *struct {
			PullRequest *pullRequestStatusDTO `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors"`
}

type graphQLPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type pullRequestStatusDTO struct {
	ID               string    `json:"id"`
	UpdatedAt        time.Time `json:"updatedAt"`
	HeadRefOID       string    `json:"headRefOid"`
	MergeStateStatus string    `json:"mergeStateStatus"`
	Mergeable        *string   `json:"mergeable"`
	MergeQueueEntry  *struct {
		ID                          string    `json:"id"`
		State                       string    `json:"state"`
		Position                    int       `json:"position"`
		EnqueuedAt                  time.Time `json:"enqueuedAt"`
		EstimatedTimeToMergeSeconds *int      `json:"estimatedTimeToMerge"`
	} `json:"mergeQueueEntry"`
	ClosingIssues graphQLConnection[pullRequestClosingIssueDTO] `json:"closingIssuesReferences"`
	Files         graphQLConnection[pullRequestFileDTO]         `json:"files"`
	ReviewThreads graphQLConnection[pullRequestReviewThreadDTO] `json:"reviewThreads"`
	Commits       struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts graphQLConnection[pullRequestCheckDTO] `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type graphQLConnection[T any] struct {
	TotalCount int             `json:"totalCount"`
	Nodes      []T             `json:"nodes"`
	PageInfo   graphQLPageInfo `json:"pageInfo"`
}

type pullRequestCheckDTO struct {
	TypeName    string     `json:"__typename"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	DetailsURL  string     `json:"detailsUrl"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	Context     string     `json:"context"`
	State       string     `json:"state"`
	TargetURL   string     `json:"targetUrl"`
	CreatedAt   *time.Time `json:"createdAt"`
}

type pullRequestReviewThreadDTO struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Line       *int   `json:"line"`
	StartLine  *int   `json:"startLine"`
}

type pullRequestClosingIssueDTO struct {
	ID         string `json:"id"`
	Number     int    `json:"number"`
	URL        string `json:"url"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

type pullRequestFileDTO struct {
	Path       string `json:"path"`
	ChangeType string `json:"changeType"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
}

func convertPullRequestStatus(dto pullRequestStatusDTO) PullRequestStatus {
	mergeable := ""
	known := false
	if dto.Mergeable != nil {
		mergeable = *dto.Mergeable
		known = mergeable != "" && !strings.EqualFold(mergeable, "UNKNOWN")
	}
	result := PullRequestStatus{
		NodeID: dto.ID, HeadSHA: dto.HeadRefOID, SourceUpdatedAt: dto.UpdatedAt,
		MergeState:         PullRequestMergeState{MergeStateStatus: dto.MergeStateStatus, Mergeable: mergeable, MergeableKnown: known},
		MergeStateCoverage: scalarCoverage(), MergeQueueCoverage: scalarCoverage(),
	}
	if dto.MergeQueueEntry != nil {
		result.MergeQueue = &PullRequestMergeQueueEntry{
			NodeID: dto.MergeQueueEntry.ID, State: dto.MergeQueueEntry.State,
			Position: dto.MergeQueueEntry.Position, EnqueuedAt: dto.MergeQueueEntry.EnqueuedAt,
			EstimatedTimeToMergeSeconds: dto.MergeQueueEntry.EstimatedTimeToMergeSeconds,
		}
	}
	result.ReviewThreads = convertReviewThreadFacet(dto.ReviewThreads)
	result.ClosingIssues = convertClosingIssueFacet(dto.ClosingIssues)
	result.Files = convertFileFacet(dto.Files)
	var checks graphQLConnection[pullRequestCheckDTO]
	if len(dto.Commits.Nodes) != 0 && dto.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		checks = dto.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts
	}
	result.Checks = convertCheckFacet(checks)
	return result
}

func scalarCoverage() FacetCoverage { return FacetCoverage{Complete: true, Fetched: 1, Total: 1} }

func coverage[T any](connection graphQLConnection[T]) FacetCoverage {
	return FacetCoverage{Complete: !connection.PageInfo.HasNextPage, Fetched: len(connection.Nodes), Total: connection.TotalCount, HasNextPage: connection.PageInfo.HasNextPage, EndCursor: connection.PageInfo.EndCursor}
}

func activePullRequestStatusFacets(status PullRequestStatus) pullRequestStatusActive {
	return pullRequestStatusActive{
		checks: !status.Checks.Coverage.Complete, threads: !status.ReviewThreads.Coverage.Complete,
		issues: !status.ClosingIssues.Coverage.Complete, files: !status.Files.Coverage.Complete,
	}
}

func mergePullRequestStatusPage(result *PullRequestStatus, page PullRequestStatus, active pullRequestStatusActive) {
	result.NodeID, result.HeadSHA, result.SourceUpdatedAt = page.NodeID, page.HeadSHA, page.SourceUpdatedAt
	result.MergeState, result.MergeStateCoverage = page.MergeState, page.MergeStateCoverage
	result.MergeQueue, result.MergeQueueCoverage = page.MergeQueue, page.MergeQueueCoverage
	if active.checks {
		mergeFacetPage(&result.Checks, page.Checks)
	}
	if active.threads {
		mergeFacetPage(&result.ReviewThreads, page.ReviewThreads)
	}
	if active.issues {
		mergeFacetPage(&result.ClosingIssues, page.ClosingIssues)
	}
	if active.files {
		mergeFacetPage(&result.Files, page.Files)
	}
}

func mergeFacetPage[T any](result *FacetResult[T], page FacetResult[T]) {
	result.Items = append(result.Items, page.Items...)
	result.Coverage = page.Coverage
	result.Coverage.Fetched = len(result.Items)
}

func convertCheckFacet(in graphQLConnection[pullRequestCheckDTO]) FacetResult[PullRequestCheck] {
	out := FacetResult[PullRequestCheck]{Items: make([]PullRequestCheck, 0, len(in.Nodes)), Coverage: coverage(in)}
	for _, item := range in.Nodes {
		check := PullRequestCheck{Kind: item.TypeName, Name: item.Name, Status: item.Status, Conclusion: item.Conclusion, DetailsURL: item.DetailsURL, StartedAt: item.StartedAt, CompletedAt: item.CompletedAt}
		if item.TypeName == "StatusContext" {
			check.Name, check.Status, check.DetailsURL, check.StartedAt = item.Context, item.State, item.TargetURL, item.CreatedAt
		}
		out.Items = append(out.Items, check)
	}
	return out
}

func convertReviewThreadFacet(in graphQLConnection[pullRequestReviewThreadDTO]) FacetResult[PullRequestReviewThread] {
	out := FacetResult[PullRequestReviewThread]{Items: make([]PullRequestReviewThread, 0, len(in.Nodes)), Coverage: coverage(in)}
	for _, item := range in.Nodes {
		out.Items = append(out.Items, PullRequestReviewThread{NodeID: item.ID, IsResolved: item.IsResolved, IsOutdated: item.IsOutdated, Path: item.Path, Line: item.Line, StartLine: item.StartLine})
	}
	return out
}

func convertClosingIssueFacet(in graphQLConnection[pullRequestClosingIssueDTO]) FacetResult[PullRequestClosingIssue] {
	out := FacetResult[PullRequestClosingIssue]{Items: make([]PullRequestClosingIssue, 0, len(in.Nodes)), Coverage: coverage(in)}
	for _, item := range in.Nodes {
		out.Items = append(out.Items, PullRequestClosingIssue{NodeID: item.ID, RepositoryFullName: item.Repository.NameWithOwner, Number: item.Number, HTMLURL: item.URL})
	}
	return out
}

func convertFileFacet(in graphQLConnection[pullRequestFileDTO]) FacetResult[PullRequestFile] {
	out := FacetResult[PullRequestFile]{Items: make([]PullRequestFile, 0, len(in.Nodes)), Coverage: coverage(in)}
	for _, item := range in.Nodes {
		out.Items = append(out.Items, PullRequestFile{Path: item.Path, ChangeType: item.ChangeType, Additions: item.Additions, Deletions: item.Deletions})
	}
	return out
}
