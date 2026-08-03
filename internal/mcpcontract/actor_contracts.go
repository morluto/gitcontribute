package mcpcontract

const (
	ToolSearchGitHubUsers      = "github.search_users"
	ToolSyncUsers              = "github.sync_users"
	ToolSyncUserSocialAccounts = "github.sync_user_social_accounts"
	ToolSyncUserOrganizations  = "github.sync_user_organizations"
	ToolSyncUserPinnedItems    = "github.sync_user_pinned_items"
	ToolSyncUserRepositories   = "github.sync_user_repositories"
	ToolSyncUserContributions  = "github.sync_user_contributions"
	ToolSearchActors           = "corpus.search_actors"
	ToolGetActors              = "corpus.get_actors"
	ToolGetActorFacets         = "corpus.get_actor_facets"
	ToolSearchContributions    = "corpus.search_contributions"
)

// ActorSelector is a discriminated exact GitHub identity selector.
type ActorSelector struct {
	Type   string `json:"type" jsonschema:"Identity selector: login or node_id"`
	Login  string `json:"login,omitempty" jsonschema:"GitHub login when type is login"`
	NodeID string `json:"node_id,omitempty" jsonschema:"GitHub GraphQL node ID when type is node_id"`
}

type SearchGitHubUsersInput struct {
	Query string `json:"query" jsonschema:"Non-empty GitHub user search query"`
	Sort  string `json:"sort,omitempty" jsonschema:"Provider ordering: best_match, followers, repositories, or joined"`
	Order string `json:"order,omitempty" jsonschema:"Provider order: asc or desc"`
	Limit int    `json:"limit,omitempty" jsonschema:"Results to return from 1 to 100"`
	Page  int    `json:"page,omitempty" jsonschema:"Provider result page from 1 to 10"`
}

type ActorIdentityOutput struct {
	ActorID    string `json:"actor_id"`
	Provider   string `json:"provider"`
	NodeID     string `json:"node_id,omitempty"`
	DatabaseID *int64 `json:"database_id,omitempty"`
	Kind       string `json:"kind"`
	Login      string `json:"login"`
}

type SearchGitHubUsersOutput struct {
	Query      string                `json:"query"`
	Total      int                   `json:"total"`
	Incomplete bool                  `json:"incomplete"`
	Page       int                   `json:"page"`
	NextPage   int                   `json:"next_page,omitempty"`
	Items      []ActorIdentityOutput `json:"items"`
	ObservedAt string                `json:"observed_at"`
	Rate       GitHubRateOutput      `json:"rate"`
}

type SyncUsersInput struct {
	Users       []ActorSelector `json:"users" jsonschema:"One to 100 exact GitHub users"`
	MaxRequests int             `json:"max_requests,omitempty" jsonschema:"Total admitted GitHub requests from 1 to 100"`
}

type SyncUserFacetInput struct {
	Users       []ActorSelector `json:"users" jsonschema:"One to 100 exact GitHub users"`
	MaxPages    int             `json:"max_pages,omitempty" jsonschema:"Maximum pages per user from 1 to 10"`
	MaxItems    int             `json:"max_items_per_user,omitempty" jsonschema:"Maximum child items per user from 1 to 1000"`
	MaxRequests int             `json:"max_requests,omitempty" jsonschema:"Total admitted GitHub requests from 1 to 1000"`
}

type SyncUserPinnedItemsInput struct {
	Users       []ActorSelector `json:"users" jsonschema:"One to 50 exact GitHub users"`
	Limit       int             `json:"limit,omitempty" jsonschema:"Pinned or showcase items per user from 1 to 6"`
	MaxRequests int             `json:"max_requests,omitempty" jsonschema:"Total admitted GitHub GraphQL requests from 1 to 100"`
}

type SyncUserRepositoriesInput struct {
	Users        []ActorSelector `json:"users" jsonschema:"One to 50 exact GitHub users"`
	Relationship string          `json:"relationship" jsonschema:"Repository relationship: owned, affiliated, or contributed"`
	Sort         string          `json:"sort,omitempty" jsonschema:"Provider ordering: created, updated, pushed, or full_name"`
	Order        string          `json:"order,omitempty" jsonschema:"Provider order: asc or desc"`
	MaxPages     int             `json:"max_pages,omitempty" jsonschema:"Maximum pages per user from 1 to 10"`
	MaxItems     int             `json:"max_items_per_user,omitempty" jsonschema:"Maximum repositories per user from 1 to 1000"`
	MaxRequests  int             `json:"max_requests,omitempty" jsonschema:"Total admitted GitHub requests from 1 to 1000"`
}

type SyncUserContributionsInput struct {
	Users              []ActorSelector `json:"users" jsonschema:"One to 20 exact GitHub users"`
	From               string          `json:"from" jsonschema:"Inclusive RFC 3339 period start"`
	To                 string          `json:"to" jsonschema:"Exclusive RFC 3339 period end no more than one year after from"`
	OrganizationNodeID string          `json:"organization_node_id,omitempty"`
	MaxRepositories    int             `json:"max_repositories,omitempty" jsonschema:"Repository groups per contribution kind from 1 to 100"`
	MaxRequests        int             `json:"max_requests,omitempty" jsonschema:"Total admitted GitHub GraphQL requests from 1 to 100"`
}

type ActorProfileOutput struct {
	Name               *string `json:"name,omitempty"`
	AvatarURL          *string `json:"avatar_url,omitempty"`
	Bio                *string `json:"bio,omitempty"`
	Company            *string `json:"company,omitempty"`
	Location           *string `json:"location,omitempty"`
	WebsiteURL         *string `json:"website_url,omitempty"`
	PublicEmail        *string `json:"public_email,omitempty"`
	TwitterUsername    *string `json:"twitter_username,omitempty"`
	Hireable           *bool   `json:"hireable,omitempty"`
	Followers          *int    `json:"followers,omitempty"`
	Following          *int    `json:"following,omitempty"`
	PublicRepositories *int    `json:"public_repositories,omitempty"`
	PublicGists        *int    `json:"public_gists,omitempty"`
	ProviderCreatedAt  string  `json:"provider_created_at,omitempty"`
}

type ActorCoverageOutput struct {
	Facet              string `json:"facet"`
	Status             string `json:"status" jsonschema:"Coverage status: complete, paginated, truncated, unknown, retryable, or unavailable"`
	ObservedAt         string `json:"observed_at,omitempty"`
	SourceUpdatedAt    string `json:"source_updated_at,omitempty"`
	AuthorizationScope string `json:"authorization_scope,omitempty"`
	PeriodFrom         string `json:"period_from,omitempty"`
	PeriodTo           string `json:"period_to,omitempty"`
	OrganizationNodeID string `json:"organization_node_id,omitempty"`
	Truncated          bool   `json:"truncated,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type ActorOutput struct {
	ActorIdentityOutput
	Profile  *ActorProfileOutput   `json:"profile,omitempty"`
	Coverage []ActorCoverageOutput `json:"coverage"`
	URI      string                `json:"uri"`
}

type SearchActorsInput struct {
	Query         string   `json:"query,omitempty"`
	Kinds         []string `json:"kinds,omitempty"`
	Sort          string   `json:"sort,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Cursor        string   `json:"cursor,omitempty"`
	SnapshotToken string   `json:"snapshot_token,omitempty"`
}

type SearchActorsOutput struct {
	Items         []ActorOutput `json:"items"`
	Total         int           `json:"total"`
	NextCursor    string        `json:"next_cursor,omitempty"`
	SnapshotToken string        `json:"snapshot_token"`
}

type GetActorsInput struct {
	Actors        []string `json:"actors" jsonschema:"One to 100 actor IDs, node IDs, or observed logins"`
	SnapshotToken string   `json:"snapshot_token,omitempty"`
}

// ActorBatchItem is intentionally small: actor reads report item state without
// embedding the catalog-wide workflow recovery union in every result schema.
type ActorBatchItem[T any] struct {
	Key     string `json:"key"`
	Status  string `json:"item_status" jsonschema:"complete, retryable, unavailable, or failed"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Value   *T     `json:"value,omitempty"`
}

type GetActorsOutput struct {
	Items         []ActorBatchItem[ActorOutput] `json:"items"`
	SnapshotToken string                        `json:"snapshot_token"`
}

type GetActorFacetsInput struct {
	Actors        []string `json:"actors" jsonschema:"One to 100 actor IDs, node IDs, or observed logins"`
	Facets        []string `json:"facets" jsonschema:"One to seven exact non-period actor facets"`
	SnapshotToken string   `json:"snapshot_token,omitempty"`
}

type ActorFacetReferenceOutput struct {
	ActorID string                `json:"actor_id"`
	Facets  []ActorCoverageOutput `json:"facets"`
	URIs    []string              `json:"uris,omitempty"`
}

type GetActorFacetsOutput struct {
	Items         []ActorBatchItem[ActorFacetReferenceOutput] `json:"items"`
	SnapshotToken string                                      `json:"snapshot_token"`
}

type SearchContributionsInput struct {
	Actors             []string        `json:"actors,omitempty"`
	Repositories       []RepositoryRef `json:"repositories,omitempty"`
	Kinds              []string        `json:"kinds,omitempty"`
	Source             string          `json:"source,omitempty" jsonschema:"Contribution source: github_profile or corpus_observation"`
	OrganizationNodeID string          `json:"organization_node_id,omitempty" jsonschema:"Exact GitHub organization node ID; empty selects global contribution periods"`
	From               string          `json:"from,omitempty"`
	To                 string          `json:"to,omitempty"`
	Sort               string          `json:"sort,omitempty"`
	Order              string          `json:"order,omitempty"`
	Limit              int             `json:"limit,omitempty"`
	Cursor             string          `json:"cursor,omitempty"`
	SnapshotToken      string          `json:"snapshot_token,omitempty"`
}

type ContributionOutput struct {
	ActorID       string `json:"actor_id"`
	Login         string `json:"login"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	OccurredAt    string `json:"occurred_at"`
	RepositoryRef string `json:"repository_ref,omitempty"`
	TargetNodeID  string `json:"target_node_id,omitempty"`
	TargetURL     string `json:"target_url,omitempty"`
	Restricted    bool   `json:"restricted"`
	Count         int    `json:"count"`
}

type SearchContributionsOutput struct {
	Items         []ContributionOutput        `json:"items"`
	Total         int                         `json:"total"`
	NextCursor    string                      `json:"next_cursor,omitempty"`
	SnapshotToken string                      `json:"snapshot_token"`
	Coverage      []ActorContributionCoverage `json:"coverage"`
}

type ActorContributionCoverage struct {
	ActorID string              `json:"actor_id"`
	Facet   ActorCoverageOutput `json:"facet"`
}
