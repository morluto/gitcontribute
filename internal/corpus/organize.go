package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/lens"
)

const (
	maxSavedNameLength     = 128
	maxCollectionBatchSize = 1000
	lensListLimit          = 1000
	collectionListLimit    = 1000
)

// LensRecord is a durable, reusable ranking definition.
type LensRecord struct {
	Definition lens.Definition
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LensList is one bounded, stable page of saved lenses.
type LensList struct {
	Records   []LensRecord
	Total     int
	Truncated bool
}

// Collection is a named set of local corpus references.
type Collection struct {
	ID          int64
	Name        string
	MemberCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CollectionList is one bounded, stable page of named collections.
type CollectionList struct {
	Collections []Collection
	Total       int
	Truncated   bool
}

// CollectionMember is one typed stable reference in a collection.
type CollectionMember struct {
	kind collectionMemberKind
	ref  string
}

type collectionMemberKind uint8

const (
	collectionRepositoryMember collectionMemberKind = iota + 1
	collectionIssueMember
	collectionPullRequestMember
	collectionThreadMember
	collectionOpportunityMember
	collectionInvestigationMember
)

func NewRepositoryCollectionMember(ref domain.RepoRef) (CollectionMember, error) {
	if !ref.IsValid() {
		return CollectionMember{}, errors.New("collection repository reference is not parsed")
	}
	return CollectionMember{kind: collectionRepositoryMember, ref: ref.String()}, nil
}

func NewThreadCollectionMember(kind domain.ThreadKind, ref domain.RepoRef, number int) (CollectionMember, error) {
	if !ref.IsValid() || number <= 0 {
		return CollectionMember{}, errors.New("collection thread reference is invalid")
	}
	var memberKind collectionMemberKind
	switch kind {
	case domain.IssueKind:
		memberKind = collectionIssueMember
	case domain.PullRequestKind:
		memberKind = collectionPullRequestMember
	default:
		return CollectionMember{}, errors.New("collection thread kind must be issue or pull_request")
	}
	return CollectionMember{kind: memberKind, ref: fmt.Sprintf("%s#%d", ref, number)}, nil
}

func NewAnyThreadCollectionMember(ref domain.RepoRef, number int) (CollectionMember, error) {
	if !ref.IsValid() || number <= 0 {
		return CollectionMember{}, errors.New("collection thread reference is invalid")
	}
	return CollectionMember{kind: collectionThreadMember, ref: fmt.Sprintf("%s#%d", ref, number)}, nil
}

func NewOpportunityCollectionMember(id string) (CollectionMember, error) {
	return newWorkflowCollectionMember(collectionOpportunityMember, id)
}

func NewInvestigationCollectionMember(id string) (CollectionMember, error) {
	return newWorkflowCollectionMember(collectionInvestigationMember, id)
}

func newWorkflowCollectionMember(kind collectionMemberKind, id string) (CollectionMember, error) {
	id, err := validateSavedText("collection workflow reference", id, 64)
	if err != nil {
		return CollectionMember{}, err
	}
	return CollectionMember{kind: kind, ref: id}, nil
}

func (m CollectionMember) Kind() string {
	switch m.kind {
	case collectionRepositoryMember:
		return "repository"
	case collectionIssueMember:
		return "issue"
	case collectionPullRequestMember:
		return "pull_request"
	case collectionThreadMember:
		return "thread"
	case collectionOpportunityMember:
		return "opportunity"
	case collectionInvestigationMember:
		return "investigation"
	default:
		return ""
	}
}

func (m CollectionMember) Ref() string { return m.ref }

func (m CollectionMember) valid() bool { return m.Kind() != "" && m.ref != "" }

// SaveLens creates or replaces a named lens after validating its scoring
// contract. Existing creation time is retained.
func (c *Corpus) SaveLens(ctx context.Context, definition lens.Definition) (*LensRecord, error) {
	name, err := validateSavedText("lens name", definition.Name, maxSavedNameLength)
	if err != nil {
		return nil, err
	}
	definition.Name = name
	if err := lens.Validate(definition); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		return nil, fmt.Errorf("encode lens: %w", err)
	}
	now := encodeTime(time.Now())
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO lenses (name, definition, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET definition=excluded.definition, updated_at=excluded.updated_at
	`, definition.Name, string(payload), now, now)
	if err != nil {
		return nil, fmt.Errorf("save lens: %w", err)
	}
	return c.GetLens(ctx, definition.Name)
}

// GetLens returns a named lens or nil when it has not been saved.
func (c *Corpus) GetLens(ctx context.Context, name string) (*LensRecord, error) {
	var payload string
	var createdAt, updatedAt int64
	err := c.db.QueryRowContext(ctx, `
		SELECT definition, created_at, updated_at FROM lenses WHERE name=?
	`, strings.TrimSpace(name)).Scan(&payload, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lens: %w", err)
	}
	var definition lens.Definition
	if err := json.Unmarshal([]byte(payload), &definition); err != nil {
		return nil, fmt.Errorf("decode lens: %w", err)
	}
	return &LensRecord{Definition: definition, CreatedAt: scanTime(createdAt), UpdatedAt: scanTime(updatedAt)}, nil
}

// ListLenses returns a bounded lens page in stable name order.
func (c *Corpus) ListLenses(ctx context.Context) (LensList, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT definition, created_at, updated_at, (SELECT COUNT(*) FROM lenses)
		FROM lenses ORDER BY name LIMIT ?
	`, lensListLimit)
	if err != nil {
		return LensList{}, fmt.Errorf("list lenses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result LensList
	for rows.Next() {
		var payload string
		var createdAt, updatedAt int64
		if err := rows.Scan(&payload, &createdAt, &updatedAt, &result.Total); err != nil {
			return LensList{}, err
		}
		var definition lens.Definition
		if err := json.Unmarshal([]byte(payload), &definition); err != nil {
			return LensList{}, fmt.Errorf("decode lens: %w", err)
		}
		result.Records = append(result.Records, LensRecord{
			Definition: definition, CreatedAt: scanTime(createdAt), UpdatedAt: scanTime(updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return LensList{}, err
	}
	result.Truncated = result.Total > len(result.Records)
	return result, nil
}

// SaveCollection creates a named collection or returns its existing identity.
func (c *Corpus) SaveCollection(ctx context.Context, name string) (*Collection, error) {
	name, err := validateSavedText("collection name", name, maxSavedNameLength)
	if err != nil {
		return nil, err
	}
	now := encodeTime(time.Now())
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO collections (name, created_at, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET updated_at=excluded.updated_at
	`, name, now, now)
	if err != nil {
		return nil, fmt.Errorf("save collection: %w", err)
	}
	return c.GetCollection(ctx, name)
}

// GetCollection returns a named collection and its current member count.
func (c *Corpus) GetCollection(ctx context.Context, name string) (*Collection, error) {
	var item Collection
	var createdAt, updatedAt int64
	err := c.db.QueryRowContext(ctx, `
		SELECT c.id, c.name, COUNT(m.ref), c.created_at, c.updated_at
		FROM collections c LEFT JOIN collection_members m ON m.collection_id=c.id
		WHERE c.name=? GROUP BY c.id
	`, strings.TrimSpace(name)).Scan(&item.ID, &item.Name, &item.MemberCount, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	item.CreatedAt = scanTime(createdAt)
	item.UpdatedAt = scanTime(updatedAt)
	return &item, nil
}

// ListCollections returns a bounded collection page in stable name order.
func (c *Corpus) ListCollections(ctx context.Context) (CollectionList, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT c.id, c.name, COUNT(m.ref), c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM collections)
		FROM collections c LEFT JOIN collection_members m ON m.collection_id=c.id
		GROUP BY c.id ORDER BY c.name LIMIT ?
	`, collectionListLimit)
	if err != nil {
		return CollectionList{}, fmt.Errorf("list collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result CollectionList
	for rows.Next() {
		var item Collection
		var createdAt, updatedAt int64
		if err := rows.Scan(&item.ID, &item.Name, &item.MemberCount, &createdAt, &updatedAt, &result.Total); err != nil {
			return CollectionList{}, err
		}
		item.CreatedAt = scanTime(createdAt)
		item.UpdatedAt = scanTime(updatedAt)
		result.Collections = append(result.Collections, item)
	}
	if err := rows.Err(); err != nil {
		return CollectionList{}, err
	}
	result.Truncated = result.Total > len(result.Collections)
	return result, nil
}

// AddCollectionMembers idempotently adds a bounded batch of typed references.
func (c *Corpus) AddCollectionMembers(ctx context.Context, collectionName string, members []CollectionMember) error {
	if len(members) == 0 {
		return nil
	}
	if len(members) > maxCollectionBatchSize {
		return fmt.Errorf("collection batch exceeds %d members", maxCollectionBatchSize)
	}
	for _, member := range members {
		if !member.valid() {
			return errors.New("collection member is not parsed")
		}
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin collection update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var collectionID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM collections WHERE name=?`, strings.TrimSpace(collectionName)).Scan(&collectionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("collection not found")
		}
		return fmt.Errorf("get collection identity: %w", err)
	}
	now := encodeTime(time.Now())
	for _, member := range members {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collection_members (collection_id, ref, kind, added_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (collection_id, kind, ref) DO NOTHING
		`, collectionID, member.Ref(), member.Kind(), now); err != nil {
			return fmt.Errorf("add collection member: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collections SET updated_at=? WHERE id=?`, now, collectionID); err != nil {
		return fmt.Errorf("update collection timestamp: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit collection update: %w", err)
	}
	return nil
}

func validateSavedText(field, value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if len(value) > limit {
		return "", fmt.Errorf("%s exceeds %d bytes", field, limit)
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", fmt.Errorf("%s contains control characters", field)
	}
	return value, nil
}
