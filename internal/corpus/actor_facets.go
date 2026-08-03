package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ActorSocialAccount struct{ Provider, URL, DisplayName string }
type ActorOrganization struct{ NodeID, Login string }
type ActorPinnedItem struct {
	Rank                                              int
	Kind, NodeID, Name, RepositoryOwner, ShowcaseKind string
}
type ActorRepositoryAffiliation struct {
	RepositoryID int64
	Relationship string
}
type ActorContributionDay struct {
	Date  string
	Count int
	Level string
}
type ActorContributionItem struct {
	Kind                    string
	OccurredAt              time.Time
	RepositoryID            *int64
	TargetNodeID, TargetURL string
	Restricted              bool
	Count                   int
}
type ActorRepositoryContributionTotal struct {
	RepositoryID int64
	Kind         string
	Count        int
}
type ActorContributionPeriodInput struct {
	ActorID                                                                                                           int64
	From, To                                                                                                          time.Time
	OrganizationNodeID, AuthorizationScope                                                                            string
	TotalCommits, TotalIssues, TotalPullRequests, TotalPullRequestReviews, TotalRepositories, RestrictedContributions *int
	Complete                                                                                                          bool
	ObservedAt, SourceUpdatedAt                                                                                       time.Time
	Days                                                                                                              []ActorContributionDay
	Items                                                                                                             []ActorContributionItem
	RepositoryTotals                                                                                                  []ActorRepositoryContributionTotal
	RawPayload                                                                                                        json.RawMessage
}

// GetActorContributionCoverage returns a complete contribution period in the
// exact organization scope that contains the requested bounded range.
func (c *Corpus) GetActorContributionCoverage(ctx context.Context, actorID int64, organizationNodeID string, from, to time.Time) (*ActorFacetCoverage, error) {
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, nil
	}
	var coverage ActorFacetCoverage
	var sourceUpdated, observedAt int64
	err := c.db.QueryRowContext(ctx, `
		SELECT source_updated_at, observation_sequence, observed_at, authorization_scope
		FROM actor_contribution_periods
		WHERE actor_id=? AND organization_node_id=? AND period_start<=? AND period_end>=? AND complete=1
		ORDER BY (period_end-period_start) ASC, source_updated_at DESC, observation_sequence DESC LIMIT 1
	`, actorID, organizationNodeID, encodeTime(from), encodeTime(to)).Scan(&sourceUpdated, &coverage.ObservationSequence, &observedAt, &coverage.AuthorizationScope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get actor contribution coverage: %w", err)
	}
	coverage.Facet = "contributions"
	coverage.Complete = true
	coverage.SourceUpdatedAt, coverage.ObservedAt = scanTime(sourceUpdated), scanTime(observedAt)
	return &coverage, nil
}

func (c *Corpus) ReplaceActorSocialAccounts(ctx context.Context, actorID int64, items []ActorSocialAccount, complete bool, observedAt, sourceUpdatedAt time.Time, authScope string, raw json.RawMessage) error {
	return c.applyActorFacetSet(ctx, actorID, "social_accounts", complete, observedAt, sourceUpdatedAt, authScope, raw, func(tx *sql.Tx, sequence int64) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM actor_social_accounts WHERE actor_id=?`, actorID); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO actor_social_accounts(actor_id,provider_name,url,display_name,source_updated_at,observation_sequence,observed_at) VALUES(?,?,?,?,?,?,?)`, actorID, item.Provider, item.URL, item.DisplayName, encodeTime(sourceUpdatedAt), sequence, encodeTime(observedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Corpus) ReplaceActorOrganizations(ctx context.Context, actorID int64, items []ActorOrganization, complete bool, observedAt, sourceUpdatedAt time.Time, authScope string, raw json.RawMessage) error {
	return c.applyActorFacetSet(ctx, actorID, "organizations", complete, observedAt, sourceUpdatedAt, authScope, raw, func(tx *sql.Tx, sequence int64) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM actor_organization_memberships WHERE actor_id=?`, actorID); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO actor_organization_memberships(actor_id,organization_node_id,organization_login,source_updated_at,observation_sequence,observed_at) VALUES(?,?,?,?,?,?)`, actorID, item.NodeID, item.Login, encodeTime(sourceUpdatedAt), sequence, encodeTime(observedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Corpus) ReplaceActorPinnedItems(ctx context.Context, actorID int64, items []ActorPinnedItem, complete bool, observedAt, sourceUpdatedAt time.Time, authScope string, raw json.RawMessage) error {
	return c.applyActorFacetSet(ctx, actorID, "pinned_items", complete, observedAt, sourceUpdatedAt, authScope, raw, func(tx *sql.Tx, sequence int64) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM actor_pinned_items WHERE actor_id=?`, actorID); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO actor_pinned_items(actor_id,rank,item_kind,target_node_id,target_name,repository_owner,showcase_kind,source_updated_at,observation_sequence,observed_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, actorID, item.Rank, item.Kind, item.NodeID, item.Name, item.RepositoryOwner, item.ShowcaseKind, encodeTime(sourceUpdatedAt), sequence, encodeTime(observedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Corpus) ReplaceActorRepositoryAffiliations(ctx context.Context, actorID int64, relationship string, items []ActorRepositoryAffiliation, complete bool, observedAt, sourceUpdatedAt time.Time, authScope string, raw json.RawMessage) error {
	facet := "repositories:" + relationship
	return c.applyActorFacetSet(ctx, actorID, facet, complete, observedAt, sourceUpdatedAt, authScope, raw, func(tx *sql.Tx, sequence int64) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM actor_repository_affiliations WHERE actor_id=? AND relationship=?`, actorID, relationship); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO actor_repository_affiliations(actor_id,repository_id,relationship,source_updated_at,observation_sequence,observed_at) VALUES(?,?,?,?,?,?)`, actorID, item.RepositoryID, relationship, encodeTime(sourceUpdatedAt), sequence, encodeTime(observedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Corpus) ApplyActorContributionPeriod(ctx context.Context, input ActorContributionPeriodInput) error {
	if input.AuthorizationScope == "" {
		input.AuthorizationScope = "public"
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	payload := input.RawPayload
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	sequence, err := c.nextSequence(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO actor_observations(actor_id,facet,source_updated_at,observation_sequence,observed_at,complete,authorization_scope,payload) VALUES(?, 'contributions', ?, ?, ?, ?, ?, ?)`, input.ActorID, encodeTime(input.SourceUpdatedAt), sequence, encodeTime(input.ObservedAt), boolToInt(input.Complete), input.AuthorizationScope, string(payload)); err != nil {
		return err
	}
	var existingComplete bool
	var existingSource, existingSequence int64
	err = tx.QueryRowContext(ctx, `SELECT complete,source_updated_at,observation_sequence FROM actor_contribution_periods WHERE actor_id=? AND period_start=? AND period_end=? AND organization_node_id=? AND authorization_scope=?`, input.ActorID, encodeTime(input.From), encodeTime(input.To), input.OrganizationNodeID, input.AuthorizationScope).Scan(&existingComplete, &existingSource, &existingSequence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && (!orderingNewer(encodeTime(input.SourceUpdatedAt), sequence, existingSource, existingSequence) || (!input.Complete && existingComplete)) {
		return tx.Commit()
	}
	var periodID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO actor_contribution_periods(actor_id,period_start,period_end,organization_node_id,authorization_scope,total_commits,total_issues,total_pull_requests,total_pull_request_reviews,total_repositories,restricted_contributions,complete,source_updated_at,observation_sequence,observed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(actor_id,period_start,period_end,organization_node_id,authorization_scope) DO UPDATE SET total_commits=excluded.total_commits,total_issues=excluded.total_issues,total_pull_requests=excluded.total_pull_requests,total_pull_request_reviews=excluded.total_pull_request_reviews,total_repositories=excluded.total_repositories,restricted_contributions=excluded.restricted_contributions,complete=excluded.complete,source_updated_at=excluded.source_updated_at,observation_sequence=excluded.observation_sequence,observed_at=excluded.observed_at RETURNING id`, input.ActorID, encodeTime(input.From), encodeTime(input.To), input.OrganizationNodeID, input.AuthorizationScope, input.TotalCommits, input.TotalIssues, input.TotalPullRequests, input.TotalPullRequestReviews, input.TotalRepositories, input.RestrictedContributions, boolToInt(input.Complete), encodeTime(input.SourceUpdatedAt), sequence, encodeTime(input.ObservedAt)).Scan(&periodID)
	if err != nil {
		return fmt.Errorf("upsert actor contribution period: %w", err)
	}
	if !input.Complete {
		return tx.Commit()
	}
	for _, statement := range []string{
		`DELETE FROM actor_contribution_days WHERE period_id=?`,
		`DELETE FROM actor_contribution_items WHERE period_id=?`,
		`DELETE FROM actor_repository_contribution_totals WHERE period_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, periodID); err != nil {
			return err
		}
	}
	for _, day := range input.Days {
		if _, err := tx.ExecContext(ctx, `INSERT INTO actor_contribution_days(period_id,contribution_date,contribution_count,contribution_level) VALUES(?,?,?,?)`, periodID, day.Date, day.Count, day.Level); err != nil {
			return err
		}
	}
	for _, item := range input.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO actor_contribution_items(period_id,contribution_kind,occurred_at,repository_id,target_node_id,target_url,restricted,count) VALUES(?,?,?,?,?,?,?,?)`, periodID, item.Kind, encodeTime(item.OccurredAt), item.RepositoryID, item.TargetNodeID, item.TargetURL, boolToInt(item.Restricted), item.Count); err != nil {
			return err
		}
	}
	for _, total := range input.RepositoryTotals {
		if _, err := tx.ExecContext(ctx, `INSERT INTO actor_repository_contribution_totals(period_id,repository_id,contribution_kind,contribution_count) VALUES(?,?,?,?)`, periodID, total.RepositoryID, total.Kind, total.Count); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *Corpus) applyActorFacetSet(ctx context.Context, actorID int64, facet string, complete bool, observedAt, sourceUpdatedAt time.Time, authScope string, raw json.RawMessage, replace func(*sql.Tx, int64) error) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if authScope == "" {
		authScope = "public"
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	sequence, err := c.nextSequence(ctx, tx)
	if err != nil {
		return err
	}
	var existingSource, existingSequence int64
	err = tx.QueryRowContext(ctx, `SELECT source_updated_at,observation_sequence FROM actor_observations WHERE actor_id=? AND facet=? AND complete=1 ORDER BY source_updated_at DESC,observation_sequence DESC LIMIT 1`, actorID, facet).Scan(&existingSource, &existingSequence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO actor_observations(actor_id,facet,source_updated_at,observation_sequence,observed_at,complete,authorization_scope,payload) VALUES(?,?,?,?,?,?,?,?)`, actorID, facet, encodeTime(sourceUpdatedAt), sequence, encodeTime(observedAt), boolToInt(complete), authScope, string(raw)); err != nil {
		return err
	}
	if !complete || (err == nil && !orderingNewer(encodeTime(sourceUpdatedAt), sequence, existingSource, existingSequence)) {
		return tx.Commit()
	}
	if err := replace(tx, sequence); err != nil {
		return fmt.Errorf("replace actor %s facet: %w", facet, err)
	}
	return tx.Commit()
}

func orderingNewer(source, sequence, currentSource, currentSequence int64) bool {
	return source > currentSource || (source == currentSource && sequence > currentSequence)
}

type ContributionSearchOptions struct {
	ActorRefs          []string
	RepositoryRefs     []string
	Kinds              []string
	OrganizationNodeID string
	From, To           time.Time
	Sort, Order        string
	Limit              int
	Cursor             string
}
type ContributionSearchItem struct {
	ActorKey, Login, Kind                  string
	OccurredAt                             time.Time
	RepositoryRef, TargetNodeID, TargetURL string
	Restricted                             bool
	Count                                  int
}
type ContributionSearchPage struct {
	Items      []ContributionSearchItem
	Total      int
	NextCursor string
}

func (c *Corpus) SearchActorContributions(ctx context.Context, opts ContributionSearchOptions) (ContributionSearchPage, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.Limit < 1 || opts.Limit > 100 {
		return ContributionSearchPage{}, errors.New("contribution search limit must be 1 to 100")
	}
	if opts.Sort == "" {
		opts.Sort = "occurred_at"
	}
	if opts.Order == "" {
		opts.Order = "desc"
	}
	if opts.Sort != "occurred_at" && opts.Sort != "repository" && opts.Sort != "type" {
		return ContributionSearchPage{}, errors.New("unsupported contribution sort")
	}
	if opts.Order != "asc" && opts.Order != "desc" {
		return ContributionSearchPage{}, errors.New("contribution order must be asc or desc")
	}
	offset := 0
	if opts.Cursor != "" {
		cursor, err := decodeCursor(opts.Cursor)
		if err != nil || cursor.Scope != "actor_contributions" || cursor.Filter != contributionFilterKey(opts) {
			return ContributionSearchPage{}, errors.New("invalid contribution cursor")
		}
		offset = int(cursor.ID)
	}
	where := " WHERE p.organization_node_id=?"
	args := []any{opts.OrganizationNodeID}
	if len(opts.ActorRefs) > 0 {
		placeholders := make([]string, len(opts.ActorRefs))
		for i := range opts.ActorRefs {
			placeholders[i] = "?"
		}
		where += ` AND (a.actor_key IN (` + strings.Join(placeholders, ",") + `) OR a.node_id IN (` + strings.Join(placeholders, ",") + `) OR a.id IN (SELECT actor_id FROM actor_aliases WHERE active=1 AND normalized_login IN (` + strings.Join(placeholders, ",") + `)))`
		for _, ref := range opts.ActorRefs {
			args = append(args, strings.TrimSpace(ref))
		}
		for _, ref := range opts.ActorRefs {
			args = append(args, strings.TrimSpace(ref))
		}
		for _, ref := range opts.ActorRefs {
			args = append(args, normalizeLogin(ref))
		}
	}
	if len(opts.Kinds) > 0 {
		p := make([]string, len(opts.Kinds))
		for i, kind := range opts.Kinds {
			p[i] = "?"
			args = append(args, kind)
		}
		where += ` AND i.contribution_kind IN (` + strings.Join(p, ",") + `)`
	}
	if len(opts.RepositoryRefs) > 0 {
		p := make([]string, len(opts.RepositoryRefs))
		for i, ref := range opts.RepositoryRefs {
			p[i] = "?"
			args = append(args, strings.ToLower(strings.TrimSpace(ref)))
		}
		where += ` AND lower(COALESCE(r.owner||'/'||r.name,'')) IN (` + strings.Join(p, ",") + `)`
	}
	if !opts.From.IsZero() {
		where += ` AND i.occurred_at>=?`
		args = append(args, encodeTime(opts.From))
	}
	if !opts.To.IsZero() {
		where += ` AND i.occurred_at<?`
		args = append(args, encodeTime(opts.To))
	}
	orderExpr := map[string]string{"occurred_at": "occurred_at", "repository": "repository_ref", "type": "contribution_kind"}[opts.Sort] + " " + strings.ToUpper(opts.Order) + ", actor_key, contribution_kind, repository_ref, target_node_id, target_url, restricted, contribution_count"
	base := ` FROM actor_contribution_items i JOIN actor_contribution_periods p ON p.id=i.period_id JOIN actors a ON a.id=p.actor_id LEFT JOIN repositories r ON r.id=i.repository_id` + where
	projection := `SELECT DISTINCT a.actor_key AS actor_key,a.current_login AS current_login,i.contribution_kind AS contribution_kind,i.occurred_at AS occurred_at,COALESCE(r.owner||'/'||r.name,'') AS repository_ref,i.target_node_id AS target_node_id,i.target_url AS target_url,i.restricted AS restricted,i.count AS contribution_count` + base
	rows, err := c.db.QueryContext(ctx, `SELECT actor_key,current_login,contribution_kind,occurred_at,repository_ref,target_node_id,target_url,restricted,contribution_count FROM (`+projection+`) ORDER BY `+orderExpr+` LIMIT ? OFFSET ?`, append(args, opts.Limit+1, offset)...)
	if err != nil {
		return ContributionSearchPage{}, err
	}
	defer func() { _ = rows.Close() }()
	page := ContributionSearchPage{}
	for rows.Next() {
		var item ContributionSearchItem
		var occurred int64
		if err := rows.Scan(&item.ActorKey, &item.Login, &item.Kind, &occurred, &item.RepositoryRef, &item.TargetNodeID, &item.TargetURL, &item.Restricted, &item.Count); err != nil {
			return page, err
		}
		item.OccurredAt = scanTime(occurred)
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > opts.Limit {
		page.Items = page.Items[:opts.Limit]
		page.NextCursor = encodeCursor(searchCursor{Scope: "actor_contributions", Filter: contributionFilterKey(opts), ID: int64(offset + opts.Limit)})
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+projection+`)`, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	return page, nil
}

func contributionFilterKey(opts ContributionSearchOptions) string {
	actors := append([]string(nil), opts.ActorRefs...)
	repositories := append([]string(nil), opts.RepositoryRefs...)
	kinds := append([]string(nil), opts.Kinds...)
	for i := range actors {
		actors[i] = normalizeLogin(actors[i])
	}
	for i := range repositories {
		repositories[i] = strings.ToLower(strings.TrimSpace(repositories[i]))
	}
	slices.Sort(actors)
	slices.Sort(repositories)
	slices.Sort(kinds)
	return strings.Join([]string{opts.Sort, opts.Order, opts.OrganizationNodeID, strings.Join(actors, ","), strings.Join(repositories, ","), strings.Join(kinds, ","), strconv.FormatInt(encodeTime(opts.From), 10), strconv.FormatInt(encodeTime(opts.To), 10)}, "|")
}
