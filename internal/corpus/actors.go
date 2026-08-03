package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const ProjectionNameActorsFTS = "actors_fts"
const ProjectionVersionActorsFTS = "actors-fts-v1"

// Actor is the current normalized projection of one GitHub actor.
type Actor struct {
	ID                  int64
	Key                 string
	Provider            string
	NodeID              string
	DatabaseID          *int64
	Kind                string
	Login               string
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Profile             *ActorProfile
	Rank                float64
}

// ActorProfile contains nullable public profile facts. Pointer scalars retain
// the distinction between a known zero/false value and an unavailable field.
type ActorProfile struct {
	Name                *string
	AvatarURL           *string
	Bio                 *string
	Company             *string
	Location            *string
	WebsiteURL          *string
	PublicEmail         *string
	TwitterUsername     *string
	Hireable            *bool
	Followers           *int
	Following           *int
	PublicRepositories  *int
	PublicGists         *int
	ProviderCreatedAt   time.Time
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	ObservedAt          time.Time
	AuthorizationScope  string
}

// ActorProfileObservation is one source-backed profile header observation.
type ActorProfileObservation struct {
	Provider           string
	NodeID             string
	DatabaseID         *int64
	Kind               string
	Login              string
	Profile            ActorProfile
	SourceUpdatedAt    time.Time
	ObservedAt         time.Time
	AuthorizationScope string
	RawPayload         json.RawMessage
}

// ActorSearchOptions scopes one local actor search page.
type ActorSearchOptions struct {
	Query  string
	Kinds  []string
	Sort   string
	Limit  int
	Cursor string
}

// ActorSearchPage is one bounded local result page.
type ActorSearchPage struct {
	Actors     []Actor
	NextCursor string
	Total      int
}

type ActorFacetCoverage struct {
	Facet               string
	Complete            bool
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	ObservedAt          time.Time
	AuthorizationScope  string
}

type ActorFacetObservation struct {
	ActorFacetCoverage
	Payload json.RawMessage
}

func (c *Corpus) GetActorFacetObservation(ctx context.Context, actorID int64, facet string) (*ActorFacetObservation, error) {
	var observation ActorFacetObservation
	var sourceUpdated, observedAt int64
	var payload string
	err := c.db.QueryRowContext(ctx, `
		SELECT facet,complete,source_updated_at,observation_sequence,observed_at,authorization_scope,payload
		FROM actor_observations WHERE actor_id=? AND facet=? ORDER BY observation_sequence DESC LIMIT 1
	`, actorID, facet).Scan(&observation.Facet, &observation.Complete, &sourceUpdated, &observation.ObservationSequence, &observedAt, &observation.AuthorizationScope, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get actor facet observation: %w", err)
	}
	observation.SourceUpdatedAt, observation.ObservedAt = scanTime(sourceUpdated), scanTime(observedAt)
	observation.Payload = json.RawMessage(payload)
	return &observation, nil
}

// ListActorFacetCoverage returns the newest observation for each requested
// facet without inferring completeness from child row counts.
func (c *Corpus) ListActorFacetCoverage(ctx context.Context, actorID int64, facets []string) (map[string]ActorFacetCoverage, error) {
	if len(facets) == 0 {
		return map[string]ActorFacetCoverage{}, nil
	}
	placeholders := make([]string, len(facets))
	args := []any{actorID}
	for index, facet := range facets {
		placeholders[index] = "?"
		args = append(args, facet)
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT ao.facet, ao.complete, ao.source_updated_at, ao.observation_sequence, ao.observed_at, ao.authorization_scope
		FROM actor_observations ao
		JOIN (
		 SELECT facet, MAX(observation_sequence) AS sequence
		 FROM actor_observations WHERE actor_id=? AND facet IN (`+strings.Join(placeholders, ",")+`)
		 GROUP BY facet
		) latest ON latest.facet=ao.facet AND latest.sequence=ao.observation_sequence
		WHERE ao.actor_id=?
	`, append(args, actorID)...)
	if err != nil {
		return nil, fmt.Errorf("list actor facet coverage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]ActorFacetCoverage, len(facets))
	for rows.Next() {
		var coverage ActorFacetCoverage
		var sourceUpdated, observedAt int64
		if err := rows.Scan(&coverage.Facet, &coverage.Complete, &sourceUpdated, &coverage.ObservationSequence, &observedAt, &coverage.AuthorizationScope); err != nil {
			return nil, err
		}
		coverage.SourceUpdatedAt, coverage.ObservedAt = scanTime(sourceUpdated), scanTime(observedAt)
		out[coverage.Facet] = coverage
	}
	return out, rows.Err()
}

// ApplyActorIdentityObservation records a search or relationship identity stub
// without claiming complete profile coverage.
func (c *Corpus) ApplyActorIdentityObservation(ctx context.Context, provider, login, nodeID string, databaseID *int64, kind, authorizationScope string, observedAt time.Time, raw json.RawMessage) (_ Actor, returnErr error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	login = strings.TrimSpace(login)
	if provider == "" || login == "" {
		return Actor{}, errors.New("actor provider and login are required")
	}
	if kind == "" {
		kind = "unknown"
	}
	if authorizationScope == "" {
		authorizationScope = "public"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Actor{}, fmt.Errorf("begin actor identity observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	input := ActorProfileObservation{Provider: provider, Login: login, NodeID: nodeID, DatabaseID: databaseID, Kind: kind, ObservedAt: observedAt}
	actorID, err := resolveActorID(ctx, tx, input, encodeTime(observedAt))
	if err != nil {
		return Actor{}, err
	}
	sequence, err := c.nextSequence(ctx, tx)
	if err != nil {
		return Actor{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_observations
		(actor_id, facet, source_updated_at, observation_sequence, observed_at, complete, authorization_scope, payload)
		VALUES (?, 'identity_search', 0, ?, ?, 0, ?, ?)
	`, actorID, sequence, encodeTime(observedAt), authorizationScope, string(raw)); err != nil {
		return Actor{}, fmt.Errorf("insert actor identity observation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE actors SET actor_key=?, node_id=NULLIF(?,''), database_id=?, kind=?, current_login=?,
		 observation_sequence=?, updated_at=?
		WHERE id=? AND NOT EXISTS (
		 SELECT 1 FROM actor_profiles WHERE actor_id=actors.id
		)
	`, actorKey(provider, nodeID, login), nodeID, databaseID, kind, login, sequence, encodeTime(observedAt), actorID); err != nil {
		return Actor{}, fmt.Errorf("advance actor identity: %w", err)
	}
	if err := activateActorAlias(ctx, tx, actorID, provider, login, nodeID, encodeTime(observedAt)); err != nil {
		return Actor{}, fmt.Errorf("upsert actor identity alias: %w", err)
	}
	if err := refreshActorFTS(ctx, tx, actorID); err != nil {
		return Actor{}, fmt.Errorf("refresh actor search row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Actor{}, fmt.Errorf("commit actor identity: %w", err)
	}
	actor, err := c.GetActorByID(ctx, actorID)
	if err != nil {
		return Actor{}, err
	}
	if actor == nil {
		return Actor{}, errors.New("actor missing after identity observation")
	}
	return *actor, nil
}

// ApplyActorProfileObservation appends the provider observation and advances
// current actor/profile projections only when the source ordering wins.
func (c *Corpus) ApplyActorProfileObservation(ctx context.Context, input ActorProfileObservation) (Actor, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Login = strings.TrimSpace(input.Login)
	input.NodeID = strings.TrimSpace(input.NodeID)
	if input.Provider == "" || input.Login == "" {
		return Actor{}, errors.New("actor provider and login are required")
	}
	if input.Kind == "" {
		input.Kind = "unknown"
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	if input.AuthorizationScope == "" {
		input.AuthorizationScope = "public"
	}
	payload := input.RawPayload
	if len(payload) == 0 {
		encoded, err := json.Marshal(input)
		if err != nil {
			return Actor{}, fmt.Errorf("encode actor observation: %w", err)
		}
		payload = encoded
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Actor{}, fmt.Errorf("begin actor observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	actorID, err := resolveActorID(ctx, tx, input, encodeTime(input.ObservedAt))
	if err != nil {
		return Actor{}, err
	}
	sequence, err := c.nextSequence(ctx, tx)
	if err != nil {
		return Actor{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_observations
		(actor_id, facet, source_updated_at, observation_sequence, observed_at, complete, authorization_scope, payload)
		VALUES (?, 'profile', ?, ?, ?, 1, ?, ?)
	`, actorID, encodeTime(input.SourceUpdatedAt), sequence, encodeTime(input.ObservedAt), input.AuthorizationScope, string(payload)); err != nil {
		return Actor{}, fmt.Errorf("insert actor observation: %w", err)
	}
	actorKey := actorKey(input.Provider, input.NodeID, input.Login)
	if _, err := tx.ExecContext(ctx, `
		UPDATE actors SET
			actor_key = ?, node_id = NULLIF(?, ''), database_id = ?, kind = ?, current_login = ?,
			source_updated_at = ?, observation_sequence = ?, updated_at = ?
		WHERE id = ? AND
			(source_updated_at < ? OR (source_updated_at = ? AND observation_sequence < ?))
	`, actorKey, input.NodeID, input.DatabaseID, input.Kind, input.Login,
		encodeTime(input.SourceUpdatedAt), sequence, encodeTime(input.ObservedAt), actorID,
		encodeTime(input.SourceUpdatedAt), encodeTime(input.SourceUpdatedAt), sequence); err != nil {
		return Actor{}, fmt.Errorf("advance actor projection: %w", err)
	}
	profile := input.Profile
	profile.SourceUpdatedAt = input.SourceUpdatedAt
	profile.ObservationSequence = sequence
	profile.ObservedAt = input.ObservedAt
	profile.AuthorizationScope = input.AuthorizationScope
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_profiles
		(actor_id, name, avatar_url, bio, company, location, website_url, public_email, twitter_username,
		 hireable, followers, following, public_repositories, public_gists, provider_created_at,
		 source_updated_at, observation_sequence, observed_at, authorization_scope)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(actor_id) DO UPDATE SET
		 name=excluded.name, avatar_url=excluded.avatar_url, bio=excluded.bio, company=excluded.company,
		 location=excluded.location, website_url=excluded.website_url, public_email=excluded.public_email,
		 twitter_username=excluded.twitter_username, hireable=excluded.hireable, followers=excluded.followers,
		 following=excluded.following, public_repositories=excluded.public_repositories, public_gists=excluded.public_gists,
		 provider_created_at=excluded.provider_created_at, source_updated_at=excluded.source_updated_at,
		 observation_sequence=excluded.observation_sequence, observed_at=excluded.observed_at,
		 authorization_scope=excluded.authorization_scope
		WHERE actor_profiles.source_updated_at < excluded.source_updated_at OR
		 (actor_profiles.source_updated_at = excluded.source_updated_at AND
		  actor_profiles.observation_sequence < excluded.observation_sequence)
	`, actorID, profile.Name, profile.AvatarURL, profile.Bio, profile.Company, profile.Location,
		profile.WebsiteURL, profile.PublicEmail, profile.TwitterUsername, nullableBool(profile.Hireable),
		profile.Followers, profile.Following, profile.PublicRepositories, profile.PublicGists,
		encodeTime(profile.ProviderCreatedAt), encodeTime(profile.SourceUpdatedAt), profile.ObservationSequence,
		encodeTime(profile.ObservedAt), profile.AuthorizationScope); err != nil {
		return Actor{}, fmt.Errorf("advance actor profile: %w", err)
	}
	if err := activateActorAlias(ctx, tx, actorID, input.Provider, input.Login, input.NodeID, encodeTime(input.ObservedAt)); err != nil {
		return Actor{}, fmt.Errorf("upsert actor alias: %w", err)
	}
	if err := refreshActorFTS(ctx, tx, actorID); err != nil {
		return Actor{}, fmt.Errorf("refresh actor search row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Actor{}, fmt.Errorf("commit actor observation: %w", err)
	}
	actor, err := c.GetActorByID(ctx, actorID)
	if err != nil {
		return Actor{}, err
	}
	if actor == nil {
		return Actor{}, errors.New("actor projection missing after commit")
	}
	return *actor, nil
}

func resolveActorID(ctx context.Context, tx *sql.Tx, input ActorProfileObservation, observedAt int64) (int64, error) {
	var id int64
	if input.NodeID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id FROM actors WHERE provider=? AND node_id=?`, input.Provider, input.NodeID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("resolve actor node id: %w", err)
		}
	}
	aliasQuery := `
		SELECT a.id FROM actor_aliases aa JOIN actors a ON a.id=aa.actor_id
		WHERE a.provider=? AND aa.normalized_login=? AND aa.active=1`
	if input.NodeID != "" {
		aliasQuery += ` AND (a.node_id IS NULL OR a.node_id='')`
	}
	aliasQuery += ` ORDER BY aa.last_observed_at DESC, a.id DESC LIMIT 1`
	err := tx.QueryRowContext(ctx, aliasQuery, input.Provider, normalizeLogin(input.Login)).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("resolve actor alias: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO actors
		(actor_key, provider, node_id, database_id, kind, current_login, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
	`, actorKey(input.Provider, input.NodeID, input.Login), input.Provider, input.NodeID,
		input.DatabaseID, input.Kind, input.Login, observedAt, observedAt)
	if err != nil {
		return 0, fmt.Errorf("insert actor: %w", err)
	}
	id, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read actor id: %w", err)
	}
	return id, nil
}

// activateActorAlias keeps a reused login historical on the old node-backed
// identity while making the newly observed node-backed identity current.
func activateActorAlias(ctx context.Context, tx *sql.Tx, actorID int64, provider, login, nodeID string, observedAt int64) error {
	normalized := normalizeLogin(login)
	active := true
	if nodeID != "" {
		var currentObservedAt int64
		err := tx.QueryRowContext(ctx, `
			SELECT aa.last_observed_at
			FROM actor_aliases aa JOIN actors a ON a.id=aa.actor_id
			WHERE aa.normalized_login=? AND aa.actor_id<>? AND aa.active=1 AND a.provider=?
			ORDER BY aa.last_observed_at DESC, aa.actor_id DESC LIMIT 1
		`, normalized, actorID, provider).Scan(&currentObservedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		active = errors.Is(err, sql.ErrNoRows) || observedAt >= currentObservedAt
		if active {
			if _, err := tx.ExecContext(ctx, `
				UPDATE actor_aliases SET active=0
				WHERE normalized_login=? AND actor_id<>?
				  AND actor_id IN (SELECT id FROM actors WHERE provider=?)
			`, normalized, actorID, provider); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO actor_aliases (actor_id, login, normalized_login, active, first_observed_at, last_observed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(actor_id, normalized_login) DO UPDATE SET
		 login=excluded.login, active=excluded.active,
		 last_observed_at=MAX(actor_aliases.last_observed_at, excluded.last_observed_at)
	`, actorID, login, normalized, boolToInt(active), observedAt, observedAt)
	return err
}

func actorKey(provider, nodeID, login string) string {
	if nodeID != "" {
		return provider + ":node:" + nodeID
	}
	return provider + ":login:" + normalizeLogin(login)
}

func normalizeLogin(login string) string { return strings.ToLower(strings.TrimSpace(login)) }

func refreshActorFTS(ctx context.Context, tx *sql.Tx, actorID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM actors_fts WHERE actor_id=?`, actorID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO actors_fts (actor_id, login, name, bio, company, location)
		SELECT a.id, a.current_login, COALESCE(p.name,''), COALESCE(p.bio,''), COALESCE(p.company,''), COALESCE(p.location,'')
		FROM actors a LEFT JOIN actor_profiles p ON p.actor_id=a.id WHERE a.id=?
	`, actorID)
	return err
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

// GetActorByID reads one current actor projection by local row identity.
func (c *Corpus) GetActorByID(ctx context.Context, id int64) (*Actor, error) {
	row := c.db.QueryRowContext(ctx, actorSelect+` WHERE a.id=?`, id)
	actor, err := scanActor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get actor: %w", err)
	}
	return &actor, nil
}

// GetActor resolves a product actor key, stable node id, or observed login.
func (c *Corpus) GetActor(ctx context.Context, ref string) (*Actor, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("actor reference is required")
	}
	row := c.db.QueryRowContext(ctx, actorSelect+`
		LEFT JOIN actor_aliases lookup_alias ON lookup_alias.actor_id=a.id
		WHERE a.actor_key=? OR a.node_id=? OR lookup_alias.normalized_login=?
		ORDER BY lookup_alias.active DESC, lookup_alias.last_observed_at DESC, a.id DESC LIMIT 1
	`, ref, ref, normalizeLogin(ref))
	actor, err := scanActor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve actor: %w", err)
	}
	return &actor, nil
}

const actorSelect = `
	SELECT a.id, a.actor_key, a.provider, COALESCE(a.node_id,''), a.database_id, a.kind, a.current_login,
	       a.source_updated_at, a.observation_sequence, a.created_at, a.updated_at,
	       p.name, p.avatar_url, p.bio, p.company, p.location, p.website_url, p.public_email,
	       p.twitter_username, p.hireable, p.followers, p.following, p.public_repositories,
	       p.public_gists, p.provider_created_at, p.source_updated_at, p.observation_sequence,
	       p.observed_at, p.authorization_scope
	FROM actors a LEFT JOIN actor_profiles p ON p.actor_id=a.id`

type actorRowScanner interface{ Scan(...any) error }

func scanActor(row actorRowScanner) (Actor, error) {
	var actor Actor
	var databaseID sql.NullInt64
	var sourceUpdated, createdAt, updatedAt int64
	var name, avatar, bio, company, location, website, email, twitter sql.NullString
	var hireable sql.NullBool
	var followers, following, repos, gists sql.NullInt64
	var providerCreated, profileSourceUpdated, profileObserved sql.NullInt64
	var profileSequence sql.NullInt64
	var authScope sql.NullString
	err := row.Scan(&actor.ID, &actor.Key, &actor.Provider, &actor.NodeID, &databaseID, &actor.Kind, &actor.Login,
		&sourceUpdated, &actor.ObservationSequence, &createdAt, &updatedAt,
		&name, &avatar, &bio, &company, &location, &website, &email, &twitter, &hireable,
		&followers, &following, &repos, &gists, &providerCreated, &profileSourceUpdated,
		&profileSequence, &profileObserved, &authScope)
	if err != nil {
		return Actor{}, err
	}
	if databaseID.Valid {
		actor.DatabaseID = &databaseID.Int64
	}
	actor.SourceUpdatedAt, actor.CreatedAt, actor.UpdatedAt = scanTime(sourceUpdated), scanTime(createdAt), scanTime(updatedAt)
	if profileObserved.Valid {
		actor.Profile = &ActorProfile{
			Name: stringPtr(name), AvatarURL: stringPtr(avatar), Bio: stringPtr(bio), Company: stringPtr(company),
			Location: stringPtr(location), WebsiteURL: stringPtr(website), PublicEmail: stringPtr(email),
			TwitterUsername: stringPtr(twitter), Hireable: boolPtrSQL(hireable), Followers: intPtrSQL(followers),
			Following: intPtrSQL(following), PublicRepositories: intPtrSQL(repos), PublicGists: intPtrSQL(gists),
			ProviderCreatedAt: scanTime(providerCreated.Int64), SourceUpdatedAt: scanTime(profileSourceUpdated.Int64),
			ObservationSequence: profileSequence.Int64, ObservedAt: scanTime(profileObserved.Int64), AuthorizationScope: authScope.String,
		}
	}
	return actor, nil
}

func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
func boolPtrSQL(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}
	return &value.Bool
}
func intPtrSQL(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

// SearchActors runs one bounded offline actor query.
func (c *Corpus) SearchActors(ctx context.Context, options ActorSearchOptions) (ActorSearchPage, error) {
	if options.Limit == 0 {
		options.Limit = 20
	}
	if options.Limit < 1 || options.Limit > 100 {
		return ActorSearchPage{}, errors.New("actor search limit must be between 1 and 100")
	}
	if options.Sort == "" {
		options.Sort = "relevance"
	}
	validSort := map[string]bool{"relevance": true, "login": true, "followers": true, "public_repositories": true, "profile_updated_at": true, "observed_at": true}
	if !validSort[options.Sort] {
		return ActorSearchPage{}, errors.New("unsupported actor sort")
	}
	offset := 0
	filterKey := actorSearchFilterKey(options)
	if options.Cursor != "" {
		cursor, err := decodeCursor(options.Cursor)
		if err != nil || cursor.Scope != "actors" || cursor.Query != options.Query || cursor.Filter != filterKey {
			return ActorSearchPage{}, errors.New("invalid actor search cursor")
		}
		offset = int(cursor.ID)
	}
	ftsQuery := literalFTSQuery(options.Query)
	from := `actors a LEFT JOIN actor_profiles p ON p.actor_id=a.id`
	where, args := ` WHERE 1=1`, []any{}
	rank := `0.0`
	if ftsQuery != "" {
		from = `actors_fts f JOIN actors a ON a.id=CAST(f.actor_id AS INTEGER) LEFT JOIN actor_profiles p ON p.actor_id=a.id`
		// FTS5 MATCH and bm25 require the virtual table name, even when its
		// row source has an alias in the FROM clause.
		where += ` AND actors_fts MATCH ?`
		args = append(args, ftsQuery)
		rank = `bm25(actors_fts, 0.0, 10.0, 5.0, 2.0, 2.0, 1.0)`
	}
	if len(options.Kinds) > 0 {
		placeholders := make([]string, len(options.Kinds))
		for i, kind := range options.Kinds {
			placeholders[i] = "?"
			args = append(args, kind)
		}
		where += ` AND a.kind IN (` + strings.Join(placeholders, ",") + `)`
	}
	order := map[string]string{
		"relevance":           rank + `, a.source_updated_at DESC, a.id`,
		"login":               `a.current_login COLLATE NOCASE, a.id`,
		"followers":           `COALESCE(p.followers,-1) DESC, a.id`,
		"public_repositories": `COALESCE(p.public_repositories,-1) DESC, a.id`,
		"profile_updated_at":  `COALESCE(p.source_updated_at,0) DESC, a.id`,
		"observed_at":         `COALESCE(p.observed_at,0) DESC, a.id`,
	}[options.Sort]
	// actorSelect includes its own FROM clause, so build the projection directly.
	query := `SELECT ` + rank + `, a.id, a.actor_key, a.provider, COALESCE(a.node_id,''), a.database_id, a.kind, a.current_login,
	 a.source_updated_at, a.observation_sequence, a.created_at, a.updated_at,
	 p.name, p.avatar_url, p.bio, p.company, p.location, p.website_url, p.public_email, p.twitter_username,
	 p.hireable, p.followers, p.following, p.public_repositories, p.public_gists, p.provider_created_at,
	 p.source_updated_at, p.observation_sequence, p.observed_at, p.authorization_scope
	 FROM ` + from + where + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, options.Limit+1, offset)
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ActorSearchPage{}, fmt.Errorf("search actors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	actors := make([]Actor, 0, options.Limit+1)
	for rows.Next() {
		var rankValue float64
		var actor Actor
		var databaseID sql.NullInt64
		var sourceUpdated, createdAt, updatedAt int64
		var name, avatar, bio, company, location, website, email, twitter sql.NullString
		var hireable sql.NullBool
		var followers, following, repos, gists sql.NullInt64
		var providerCreated, profileSourceUpdated, profileSequence, profileObserved sql.NullInt64
		var authScope sql.NullString
		if err := rows.Scan(&rankValue, &actor.ID, &actor.Key, &actor.Provider, &actor.NodeID, &databaseID, &actor.Kind, &actor.Login,
			&sourceUpdated, &actor.ObservationSequence, &createdAt, &updatedAt, &name, &avatar, &bio, &company, &location,
			&website, &email, &twitter, &hireable, &followers, &following, &repos, &gists, &providerCreated,
			&profileSourceUpdated, &profileSequence, &profileObserved, &authScope); err != nil {
			return ActorSearchPage{}, err
		}
		actor.Rank = rankValue
		if databaseID.Valid {
			actor.DatabaseID = &databaseID.Int64
		}
		actor.SourceUpdatedAt, actor.CreatedAt, actor.UpdatedAt = scanTime(sourceUpdated), scanTime(createdAt), scanTime(updatedAt)
		if profileObserved.Valid {
			actor.Profile = &ActorProfile{Name: stringPtr(name), AvatarURL: stringPtr(avatar), Bio: stringPtr(bio), Company: stringPtr(company), Location: stringPtr(location), WebsiteURL: stringPtr(website), PublicEmail: stringPtr(email), TwitterUsername: stringPtr(twitter), Hireable: boolPtrSQL(hireable), Followers: intPtrSQL(followers), Following: intPtrSQL(following), PublicRepositories: intPtrSQL(repos), PublicGists: intPtrSQL(gists), ProviderCreatedAt: scanTime(providerCreated.Int64), SourceUpdatedAt: scanTime(profileSourceUpdated.Int64), ObservationSequence: profileSequence.Int64, ObservedAt: scanTime(profileObserved.Int64), AuthorizationScope: authScope.String}
		}
		actors = append(actors, actor)
	}
	if err := rows.Err(); err != nil {
		return ActorSearchPage{}, err
	}
	page := ActorSearchPage{Actors: actors}
	if len(page.Actors) > options.Limit {
		page.Actors = page.Actors[:options.Limit]
		page.NextCursor = encodeCursor(searchCursor{Scope: "actors", Query: options.Query, Filter: filterKey, ID: int64(offset + options.Limit)})
	}
	countQuery := `SELECT COUNT(*) FROM ` + from + where
	countArgs := args[:len(args)-2]
	if err := c.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&page.Total); err != nil {
		return ActorSearchPage{}, fmt.Errorf("count actors: %w", err)
	}
	return page, nil
}

func actorSearchFilterKey(options ActorSearchOptions) string {
	kinds := append([]string(nil), options.Kinds...)
	slices.Sort(kinds)
	return options.Sort + "|" + strings.Join(kinds, ",")
}
