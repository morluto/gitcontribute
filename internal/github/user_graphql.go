package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v89/github"
)

const userOrganizationsQuery = `query UserOrganizations($login:String!,$first:Int!,$after:String){user(login:$login){organizations(first:$first,after:$after){totalCount nodes{id login avatarUrl} pageInfo{hasNextPage endCursor}}}}`
const userPinnedItemsQuery = `query UserPinnedItems($login:String!,$first:Int!){user(login:$login){itemShowcase{hasPinnedItems items(first:$first){totalCount nodes{__typename ... on Repository{id name owner{login}} ... on Gist{id name}} pageInfo{hasNextPage endCursor}}}}}`
const userContributionsQuery = `query UserContributions($login:String!,$from:DateTime!,$to:DateTime!,$organizationID:ID,$maxRepositories:Int!){user(login:$login){contributionsCollection(from:$from,to:$to,organizationID:$organizationID){startedAt endedAt restrictedContributionsCount totalCommitContributions totalIssueContributions totalPullRequestContributions totalPullRequestReviewContributions totalRepositoryContributions contributionCalendar{weeks{contributionDays{date contributionCount contributionLevel}}} commitContributionsByRepository(maxRepositories:$maxRepositories){repository{id nameWithOwner} contributions(first:100){nodes{occurredAt commitCount isRestricted} pageInfo{hasNextPage}}} issueContributions(first:100){nodes{occurredAt isRestricted issue{id url repository{id nameWithOwner}}} pageInfo{hasNextPage}} pullRequestContributions(first:100){nodes{occurredAt isRestricted pullRequest{id url repository{id nameWithOwner}}} pageInfo{hasNextPage}} pullRequestReviewContributions(first:100){nodes{occurredAt isRestricted pullRequest{id url repository{id nameWithOwner}} pullRequestReview{id url}} pageInfo{hasNextPage}} repositoryContributions(first:100){nodes{occurredAt isRestricted repository{id url nameWithOwner}} pageInfo{hasNextPage}}}}}`
const userContributedRepositoriesQuery = `query UserContributedRepositories($login:String!,$first:Int!,$after:String){user(login:$login){repositoriesContributedTo(first:$first,after:$after,includeUserRepositories:true,contributionTypes:[COMMIT,ISSUE,PULL_REQUEST,REPOSITORY]){nodes{id name nameWithOwner description defaultBranchRef{name} isFork isArchived isPrivate stargazerCount forkCount issues(states:OPEN){totalCount} primaryLanguage{name} licenseInfo{spdxId} repositoryTopics(first:20){nodes{topic{name}}} owner{login} createdAt updatedAt pushedAt} pageInfo{hasNextPage endCursor}}}}`

type graphQLErrorDTO struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (c *Client) ListUserOrganizations(ctx context.Context, login string, opts CursorPageOptions) (ListResult[OrganizationIdentity], error) {
	first := opts.First
	if first <= 0 {
		first = 100
	}
	if first > 100 {
		first = 100
	}
	var envelope struct {
		Data struct {
			User *struct {
				Organizations graphQLConnection[struct {
					ID        string `json:"id"`
					Login     string `json:"login"`
					AvatarURL string `json:"avatarUrl"`
				}] `json:"organizations"`
			} `json:"user"`
		} `json:"data"`
		Errors []graphQLErrorDTO `json:"errors"`
	}
	resp, err := c.graphQLRead(ctx, userOrganizationsQuery, map[string]any{"login": login, "first": first, "after": optionalGraphQLCursor(opts.After)}, &envelope)
	if err != nil {
		return ListResult[OrganizationIdentity]{}, err
	}
	if len(envelope.Errors) > 0 {
		return ListResult[OrganizationIdentity]{}, graphQLErrors(envelope.Errors)
	}
	if envelope.Data.User == nil {
		return ListResult[OrganizationIdentity]{}, &NotFoundError{Resource: "user " + login}
	}
	items := make([]OrganizationIdentity, 0, len(envelope.Data.User.Organizations.Nodes))
	for _, node := range envelope.Data.User.Organizations.Nodes {
		items = append(items, OrganizationIdentity{NodeID: node.ID, Login: node.Login, AvatarURL: node.AvatarURL})
	}
	page := PageInfo{HasNext: envelope.Data.User.Organizations.PageInfo.HasNextPage, EndCursor: envelope.Data.User.Organizations.PageInfo.EndCursor}
	return ListResult[OrganizationIdentity]{Items: items, Page: page, Rate: rateInfo(resp.Rate)}, nil
}

func (c *Client) GetUserPinnedItems(ctx context.Context, login string, limit int) (PinnedItemsResult, error) {
	if limit <= 0 {
		limit = 6
	}
	if limit > 6 {
		limit = 6
	}
	type nodeDTO struct {
		TypeName string `json:"__typename"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Owner    *struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	var envelope struct {
		Data struct {
			User *struct {
				ItemShowcase struct {
					HasPinnedItems bool                       `json:"hasPinnedItems"`
					Items          graphQLConnection[nodeDTO] `json:"items"`
				} `json:"itemShowcase"`
			} `json:"user"`
		} `json:"data"`
		Errors []graphQLErrorDTO `json:"errors"`
	}
	resp, err := c.graphQLRead(ctx, userPinnedItemsQuery, map[string]any{"login": login, "first": limit}, &envelope)
	if err != nil {
		return PinnedItemsResult{}, err
	}
	if len(envelope.Errors) > 0 {
		return PinnedItemsResult{}, graphQLErrors(envelope.Errors)
	}
	if envelope.Data.User == nil {
		return PinnedItemsResult{}, &NotFoundError{Resource: "user " + login}
	}
	items := make([]PinnedItem, 0, len(envelope.Data.User.ItemShowcase.Items.Nodes))
	for index, node := range envelope.Data.User.ItemShowcase.Items.Nodes {
		owner := ""
		if node.Owner != nil {
			owner = node.Owner.Login
		}
		items = append(items, PinnedItem{Kind: strings.ToLower(node.TypeName), NodeID: node.ID, Name: node.Name, RepositoryOwner: owner, Rank: index + 1})
	}
	showcase := "popular"
	if envelope.Data.User.ItemShowcase.HasPinnedItems {
		showcase = "pinned"
	}
	return PinnedItemsResult{Items: items, ShowcaseKind: showcase, Coverage: coverage(envelope.Data.User.ItemShowcase.Items), Rate: rateInfo(resp.Rate)}, nil
}

func (c *Client) listUserContributedRepositories(ctx context.Context, login string, opts UserRepositoryOptions) (ListResult[Repository], error) {
	first := opts.PerPage
	if first <= 0 {
		first = 100
	}
	if first > 100 {
		first = 100
	}
	type repoDTO struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		NameWithOwner    string `json:"nameWithOwner"`
		Description      string `json:"description"`
		DefaultBranchRef *struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
		IsFork     bool `json:"isFork"`
		IsArchived bool `json:"isArchived"`
		IsPrivate  bool `json:"isPrivate"`
		Stars      int  `json:"stargazerCount"`
		Forks      int  `json:"forkCount"`
		Issues     struct {
			Total int `json:"totalCount"`
		} `json:"issues"`
		PrimaryLanguage *struct {
			Name string `json:"name"`
		} `json:"primaryLanguage"`
		LicenseInfo *struct {
			SPDXID string `json:"spdxId"`
		} `json:"licenseInfo"`
		Topics struct {
			Nodes []struct {
				Topic struct {
					Name string `json:"name"`
				} `json:"topic"`
			} `json:"nodes"`
		} `json:"repositoryTopics"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		CreatedAt time.Time  `json:"createdAt"`
		UpdatedAt time.Time  `json:"updatedAt"`
		PushedAt  *time.Time `json:"pushedAt"`
	}
	var envelope struct {
		Data struct {
			User *struct {
				Repositories graphQLConnection[repoDTO] `json:"repositoriesContributedTo"`
			} `json:"user"`
		} `json:"data"`
		Errors []graphQLErrorDTO `json:"errors"`
	}
	resp, err := c.graphQLRead(ctx, userContributedRepositoriesQuery, map[string]any{"login": login, "first": first, "after": optionalGraphQLCursor(opts.After)}, &envelope)
	if err != nil {
		return ListResult[Repository]{}, err
	}
	if len(envelope.Errors) > 0 {
		return ListResult[Repository]{}, graphQLErrors(envelope.Errors)
	}
	if envelope.Data.User == nil {
		return ListResult[Repository]{}, &NotFoundError{Resource: "user " + login}
	}
	items := make([]Repository, 0, len(envelope.Data.User.Repositories.Nodes))
	for _, node := range envelope.Data.User.Repositories.Nodes {
		repository := Repository{NodeID: node.ID, Owner: node.Owner.Login, Name: node.Name, FullName: node.NameWithOwner, Description: node.Description, Fork: node.IsFork, Archived: node.IsArchived, Private: node.IsPrivate, Stars: node.Stars, Forks: node.Forks, OpenIssues: node.Issues.Total, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt, PushedAt: node.PushedAt}
		if node.DefaultBranchRef != nil {
			repository.DefaultBranch = node.DefaultBranchRef.Name
		}
		if node.PrimaryLanguage != nil {
			repository.Language = node.PrimaryLanguage.Name
		}
		if node.LicenseInfo != nil {
			repository.License = node.LicenseInfo.SPDXID
		}
		for _, topic := range node.Topics.Nodes {
			repository.Topics = append(repository.Topics, topic.Topic.Name)
		}
		items = append(items, repository)
	}
	page := PageInfo{HasNext: envelope.Data.User.Repositories.PageInfo.HasNextPage, EndCursor: envelope.Data.User.Repositories.PageInfo.EndCursor}
	return ListResult[Repository]{Items: items, Page: page, Rate: rateInfo(resp.Rate)}, nil
}

func (c *Client) GetUserContributions(ctx context.Context, login string, opts UserContributionOptions) (UserContributionCollection, error) {
	if opts.MaxRepositories <= 0 {
		opts.MaxRepositories = 25
	}
	if opts.MaxRepositories > 100 {
		opts.MaxRepositories = 100
	}
	type contributionNode struct {
		OccurredAt time.Time `json:"occurredAt"`
		Restricted bool      `json:"isRestricted"`
		Issue      *struct {
			ID         string `json:"id"`
			URL        string `json:"url"`
			Repository struct {
				ID            string `json:"id"`
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"issue"`
		PullRequest *struct {
			ID         string `json:"id"`
			URL        string `json:"url"`
			Repository struct {
				ID            string `json:"id"`
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"pullRequest"`
		PullRequestReview *struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"pullRequestReview"`
		Repository *struct {
			ID            string `json:"id"`
			URL           string `json:"url"`
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	}
	type contributionConnection struct {
		Nodes    []contributionNode `json:"nodes"`
		PageInfo graphQLPageInfo    `json:"pageInfo"`
	}
	type commitGroup struct {
		Repository struct {
			ID            string `json:"id"`
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
		Contributions struct {
			Nodes []struct {
				OccurredAt  time.Time `json:"occurredAt"`
				CommitCount int       `json:"commitCount"`
				Restricted  bool      `json:"isRestricted"`
			} `json:"nodes"`
			PageInfo graphQLPageInfo `json:"pageInfo"`
		} `json:"contributions"`
	}
	var envelope struct {
		Data struct {
			User *struct {
				Contributions struct {
					StartedAt    time.Time `json:"startedAt"`
					EndedAt      time.Time `json:"endedAt"`
					Restricted   int       `json:"restrictedContributionsCount"`
					TotalCommits int       `json:"totalCommitContributions"`
					TotalIssues  int       `json:"totalIssueContributions"`
					TotalPRs     int       `json:"totalPullRequestContributions"`
					TotalReviews int       `json:"totalPullRequestReviewContributions"`
					TotalRepos   int       `json:"totalRepositoryContributions"`
					Calendar     struct {
						Weeks []struct {
							Days []struct {
								Date  string `json:"date"`
								Count int    `json:"contributionCount"`
								Level string `json:"contributionLevel"`
							} `json:"contributionDays"`
						} `json:"weeks"`
					} `json:"contributionCalendar"`
					Commits      []commitGroup          `json:"commitContributionsByRepository"`
					Issues       contributionConnection `json:"issueContributions"`
					PRs          contributionConnection `json:"pullRequestContributions"`
					Reviews      contributionConnection `json:"pullRequestReviewContributions"`
					Repositories contributionConnection `json:"repositoryContributions"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []graphQLErrorDTO `json:"errors"`
	}
	resp, err := c.graphQLRead(ctx, userContributionsQuery, map[string]any{"login": login, "from": opts.From.UTC().Format(time.RFC3339), "to": opts.To.UTC().Format(time.RFC3339), "organizationID": emptyToNil(opts.OrganizationNodeID), "maxRepositories": opts.MaxRepositories}, &envelope)
	if err != nil {
		return UserContributionCollection{}, err
	}
	if len(envelope.Errors) > 0 {
		return UserContributionCollection{}, graphQLErrors(envelope.Errors)
	}
	if envelope.Data.User == nil {
		return UserContributionCollection{}, &NotFoundError{Resource: "user " + login}
	}
	d := envelope.Data.User.Contributions
	out := UserContributionCollection{StartedAt: d.StartedAt, EndedAt: d.EndedAt, TotalCommits: d.TotalCommits, TotalIssues: d.TotalIssues, TotalPullRequests: d.TotalPRs, TotalPullRequestReviews: d.TotalReviews, TotalRepositories: d.TotalRepos, RestrictedContributions: d.Restricted, Complete: true, Rate: rateInfo(resp.Rate)}
	if len(d.Commits) >= opts.MaxRepositories {
		out.Complete = false
	}
	for _, week := range d.Calendar.Weeks {
		for _, day := range week.Days {
			out.Days = append(out.Days, ContributionDay{Date: day.Date, Count: day.Count, Level: day.Level})
		}
	}
	for _, group := range d.Commits {
		total := 0
		for _, node := range group.Contributions.Nodes {
			total += node.CommitCount
			out.Items = append(out.Items, UserContribution{Kind: "commit", OccurredAt: node.OccurredAt, RepositoryNodeID: group.Repository.ID, RepositoryNameOwner: group.Repository.NameWithOwner, Restricted: node.Restricted, Count: node.CommitCount})
		}
		out.RepositoryTotals = append(out.RepositoryTotals, RepositoryContributionTotal{RepositoryNodeID: group.Repository.ID, RepositoryNameOwner: group.Repository.NameWithOwner, Kind: "commit", Count: total})
		out.Complete = out.Complete && !group.Contributions.PageInfo.HasNextPage
	}
	appendConnection := func(kind string, connection contributionConnection) {
		for _, node := range connection.Nodes {
			item := UserContribution{Kind: kind, OccurredAt: node.OccurredAt, Restricted: node.Restricted, Count: 1}
			switch {
			case node.Issue != nil:
				item.RepositoryNodeID = node.Issue.Repository.ID
				item.RepositoryNameOwner = node.Issue.Repository.NameWithOwner
				item.TargetNodeID = node.Issue.ID
				item.TargetURL = node.Issue.URL
			case node.PullRequest != nil:
				item.RepositoryNodeID = node.PullRequest.Repository.ID
				item.RepositoryNameOwner = node.PullRequest.Repository.NameWithOwner
				item.TargetNodeID = node.PullRequest.ID
				item.TargetURL = node.PullRequest.URL
				if node.PullRequestReview != nil {
					item.TargetNodeID = node.PullRequestReview.ID
					item.TargetURL = node.PullRequestReview.URL
				}
			case node.Repository != nil:
				item.RepositoryNodeID = node.Repository.ID
				item.RepositoryNameOwner = node.Repository.NameWithOwner
				item.TargetNodeID = node.Repository.ID
				item.TargetURL = node.Repository.URL
			}
			out.Items = append(out.Items, item)
		}
		if connection.PageInfo.HasNextPage {
			out.Complete = false
		}
	}
	appendConnection("issue", d.Issues)
	appendConnection("pull_request", d.PRs)
	appendConnection("pull_request_review", d.Reviews)
	appendConnection("repository", d.Repositories)
	return out, nil
}

func (c *Client) graphQLRead(ctx context.Context, query string, variables map[string]any, out any) (*gh.Response, error) {
	req, err := c.gh.NewRequest(ctx, http.MethodPost, c.graphQLURL, graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}
	req = markReplayableRead(req)
	resp, err := c.gh.Do(req, out)
	if err != nil {
		return resp, classifyError(err)
	}
	return resp, nil
}
func graphQLErrors(items []graphQLErrorDTO) error {
	messages := make([]string, len(items))
	for i, item := range items {
		messages[i] = item.Message
	}
	return fmt.Errorf("github graphql: %s", strings.Join(messages, "; "))
}
func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
