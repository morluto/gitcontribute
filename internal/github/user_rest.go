package github

import (
	"context"
	"strings"

	gh "github.com/google/go-github/v89/github"
)

// UserSearcher discovers GitHub account identities without hydrating every
// result profile.
type UserSearcher interface {
	SearchUsers(context.Context, UserSearchOptions) (UserSearchResult, error)
}

// UserProfileReader reads one exact public or viewer-visible profile header.
type UserProfileReader interface {
	GetUser(context.Context, string) (Actor, RateInfo, error)
}

// UserSocialAccountReader reads one bounded page of public social accounts.
type UserSocialAccountReader interface {
	ListUserSocialAccounts(context.Context, string, PageOptions) (ListResult[SocialAccount], error)
}

// UserRepositoryReader reads one bounded page of repositories related to a
// user. Contributed relationships require the GraphQL capability below.
type UserRepositoryReader interface {
	ListUserRepositories(context.Context, string, UserRepositoryOptions) (ListResult[Repository], error)
}

type UserOrganizationReader interface {
	ListUserOrganizations(context.Context, string, CursorPageOptions) (ListResult[OrganizationIdentity], error)
}

type UserPinnedItemReader interface {
	GetUserPinnedItems(context.Context, string, int) (PinnedItemsResult, error)
}

type UserContributionReader interface {
	GetUserContributions(context.Context, string, UserContributionOptions) (UserContributionCollection, error)
}

// SearchUsers reads one page from GitHub's user Search API. Search results are
// identity stubs and intentionally do not trigger per-result profile reads.
func (c *Client) SearchUsers(ctx context.Context, opts UserSearchOptions) (UserSearchResult, error) {
	result, resp, err := c.gh.Search.Users(ctx, opts.Query, &gh.SearchOptions{
		Sort: opts.Sort, Order: opts.Order,
		ListOptions: gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	})
	if err != nil {
		return UserSearchResult{}, classifyError(err)
	}
	items := make([]Actor, 0, len(result.Users))
	for _, user := range result.Users {
		items = append(items, convertActor(user))
	}
	return UserSearchResult{Total: result.GetTotal(), Incomplete: result.GetIncompleteResults(), Items: items, Page: pageInfo(resp), Rate: rateInfo(resp.Rate)}, nil
}

// GetUser reads one exact profile header.
func (c *Client) GetUser(ctx context.Context, login string) (Actor, RateInfo, error) {
	user, resp, err := c.gh.Users.Get(ctx, login)
	if err != nil {
		return Actor{}, RateInfo{}, classifyError(err)
	}
	return convertActor(user), rateInfo(resp.Rate), nil
}

// ListUserSocialAccounts reads one bounded social-account page.
func (c *Client) ListUserSocialAccounts(ctx context.Context, login string, opts PageOptions) (ListResult[SocialAccount], error) {
	accounts, resp, err := c.gh.Users.ListUserSocialAccounts(ctx, login, &gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage})
	if err != nil {
		return ListResult[SocialAccount]{}, classifyError(err)
	}
	items := make([]SocialAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		items = append(items, SocialAccount{Provider: account.GetProvider(), URL: account.GetURL()})
	}
	return ListResult[SocialAccount]{Items: items, Page: pageInfo(resp), Rate: rateInfo(resp.Rate)}, nil
}

// ListUserRepositories reads one bounded owned or affiliated repository page.
func (c *Client) ListUserRepositories(ctx context.Context, login string, opts UserRepositoryOptions) (ListResult[Repository], error) {
	if opts.Relationship == "contributed" {
		return c.listUserContributedRepositories(ctx, login, opts)
	}
	relation := opts.Relationship
	switch relation {
	case "owned":
		relation = "owner"
	case "affiliated":
		relation = "member"
	}
	repositories, resp, err := c.gh.Repositories.ListByUser(ctx, login, &gh.RepositoryListByUserOptions{
		Type: relation, Sort: opts.Sort, Direction: opts.Direction,
		ListOptions: gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	})
	if err != nil {
		return ListResult[Repository]{}, classifyError(err)
	}
	items := make([]Repository, 0, len(repositories))
	for _, repository := range repositories {
		items = append(items, convertRepository(repository))
	}
	return ListResult[Repository]{Items: items, Page: pageInfo(resp), Rate: rateInfo(resp.Rate)}, nil
}

func convertActor(user *gh.User) Actor {
	if user == nil {
		return Actor{}
	}
	actor := Actor{
		Login: user.GetLogin(), ID: user.GetID(), NodeID: user.GetNodeID(), Kind: strings.ToLower(user.GetType()),
		AvatarURL: user.AvatarURL, Name: user.Name, Bio: user.Bio, Company: user.Company, Location: user.Location,
		WebsiteURL: user.Blog, PublicEmail: user.Email, TwitterUsername: user.TwitterUsername, Hireable: user.Hireable,
		Followers: user.Followers, Following: user.Following, PublicRepositories: user.PublicRepos, PublicGists: user.PublicGists,
	}
	if user.CreatedAt != nil {
		actor.CreatedAt = user.CreatedAt.Time
	}
	if user.UpdatedAt != nil {
		actor.UpdatedAt = user.UpdatedAt.Time
	}
	if actor.Kind == "" {
		actor.Kind = "unknown"
	}
	return actor
}
