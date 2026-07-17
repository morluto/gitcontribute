package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/lens"
)

const (
	maxSavedNameLength     = 128
	maxCollectionBatchSize = 1000
	maxCollectionRefLength = 2048
)

// LensRecord is a durable, reusable ranking definition.
type LensRecord struct {
	Definition lens.Definition
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Collection is a named set of local corpus references.
type Collection struct {
	ID          int64
	Name        string
	MemberCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CollectionMember is one typed stable reference in a collection.
type CollectionMember struct {
	Ref     string
	Kind    string
	AddedAt time.Time
}

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

// ListLenses returns saved lenses in stable name order.
func (c *Corpus) ListLenses(ctx context.Context) ([]LensRecord, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT definition, created_at, updated_at FROM lenses ORDER BY name LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("list lenses: %w", err)
	}
	defer rows.Close()
	var records []LensRecord
	for rows.Next() {
		var payload string
		var createdAt, updatedAt int64
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var definition lens.Definition
		if err := json.Unmarshal([]byte(payload), &definition); err != nil {
			return nil, fmt.Errorf("decode lens: %w", err)
		}
		records = append(records, LensRecord{
			Definition: definition, CreatedAt: scanTime(createdAt), UpdatedAt: scanTime(updatedAt),
		})
	}
	return records, rows.Err()
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

// ListCollections returns collections in stable name order.
func (c *Corpus) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT c.id, c.name, COUNT(m.ref), c.created_at, c.updated_at
		FROM collections c LEFT JOIN collection_members m ON m.collection_id=c.id
		GROUP BY c.id ORDER BY c.name LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	var collections []Collection
	for rows.Next() {
		var item Collection
		var createdAt, updatedAt int64
		if err := rows.Scan(&item.ID, &item.Name, &item.MemberCount, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = scanTime(createdAt)
		item.UpdatedAt = scanTime(updatedAt)
		collections = append(collections, item)
	}
	return collections, rows.Err()
}

// AddCollectionMembers idempotently adds a bounded batch of typed references.
func (c *Corpus) AddCollectionMembers(ctx context.Context, collectionName string, members []CollectionMember) error {
	if len(members) == 0 {
		return nil
	}
	if len(members) > maxCollectionBatchSize {
		return fmt.Errorf("collection batch exceeds %d members", maxCollectionBatchSize)
	}
	validated := make([]CollectionMember, len(members))
	for i, member := range members {
		ref, err := validateSavedText("collection reference", member.Ref, maxCollectionRefLength)
		if err != nil {
			return err
		}
		kind, err := validateSavedText("collection member kind", member.Kind, maxSavedNameLength)
		if err != nil {
			return err
		}
		validated[i] = CollectionMember{Ref: ref, Kind: kind}
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
	for _, member := range validated {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collection_members (collection_id, ref, kind, added_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (collection_id, kind, ref) DO NOTHING
		`, collectionID, member.Ref, member.Kind, now); err != nil {
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

// ListCollectionMembers returns members in stable kind and reference order.
func (c *Corpus) ListCollectionMembers(ctx context.Context, collectionName string) ([]CollectionMember, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT m.ref, m.kind, m.added_at FROM collection_members m
		JOIN collections c ON c.id=m.collection_id
		WHERE c.name=? ORDER BY m.kind, m.ref LIMIT 10000
	`, strings.TrimSpace(collectionName))
	if err != nil {
		return nil, fmt.Errorf("list collection members: %w", err)
	}
	defer rows.Close()
	var members []CollectionMember
	for rows.Next() {
		var member CollectionMember
		var addedAt int64
		if err := rows.Scan(&member.Ref, &member.Kind, &addedAt); err != nil {
			return nil, err
		}
		member.AddedAt = scanTime(addedAt)
		members = append(members, member)
	}
	return members, rows.Err()
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
