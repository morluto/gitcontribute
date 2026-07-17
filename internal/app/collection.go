package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
)

// CreateCollection creates a named collection.
func (s *Service) CreateCollection(ctx context.Context, name string) (*cli.CollectionResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("collection name is required")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	col, err := c.SaveCollection(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	return collectionResult(col), nil
}

// AddCollectionMembers idempotently adds typed references to a collection.
func (s *Service) AddCollectionMembers(ctx context.Context, name string, members []cli.CollectionMember) (*cli.CollectionResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("collection name is required")
	}
	if len(members) == 0 {
		return nil, errors.New("at least one member is required")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	cm := make([]corpus.CollectionMember, len(members))
	for i, m := range members {
		cm[i] = corpus.CollectionMember{Kind: m.Kind, Ref: m.Ref}
	}
	if err := c.AddCollectionMembers(ctx, name, cm); err != nil {
		return nil, fmt.Errorf("add collection members: %w", err)
	}

	col, err := c.GetCollection(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	if col == nil {
		return nil, fmt.Errorf("collection %q not found after update", name)
	}
	return collectionResult(col), nil
}

// ListCollections returns all named collections.
func (s *Service) ListCollections(ctx context.Context) (*cli.CollectionListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	cols, err := c.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	result := &cli.CollectionListResult{Collections: make([]cli.CollectionResult, len(cols))}
	for i, col := range cols {
		result.Collections[i] = *collectionResult(&col)
	}
	return result, nil
}

func collectionResult(c *corpus.Collection) *cli.CollectionResult {
	return &cli.CollectionResult{
		Name:        c.Name,
		MemberCount: c.MemberCount,
		CreatedAt:   formatTime(c.CreatedAt),
		UpdatedAt:   formatTime(c.UpdatedAt),
	}
}
