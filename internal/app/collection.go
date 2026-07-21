package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
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
		if err := validateCollectionMember(m); err != nil {
			return nil, fmt.Errorf("member %d: %w", i+1, err)
		}
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

func validateCollectionMember(member cli.CollectionMember) error {
	kind := strings.TrimSpace(member.Kind)
	ref := strings.TrimSpace(member.Ref)
	if ref == "" {
		return errors.New("collection member reference is required")
	}
	switch kind {
	case "repository":
		return validateCollectionRepoRef(ref)
	case "issue", "pull_request", "thread":
		return validateCollectionThreadRef(kind, ref)
	case "opportunity", "investigation":
		if len(ref) > 64 {
			return fmt.Errorf("invalid %s reference %q: exceeds 64 bytes", kind, ref)
		}
		if _, err := uuid.Parse(ref); err != nil {
			return fmt.Errorf("invalid %s reference %q: expected durable id", kind, ref)
		}
		return nil
	default:
		return fmt.Errorf("unsupported collection member kind %q", kind)
	}
}

func validateCollectionThreadRef(kind, ref string) error {
	if strings.Count(ref, "#") != 1 {
		return fmt.Errorf("invalid %s reference %q: expected OWNER/REPO#NUMBER", kind, ref)
	}
	repoRef, numberText, _ := strings.Cut(ref, "#")
	if err := validateCollectionRepoRef(repoRef); err != nil {
		return fmt.Errorf("invalid %s reference %q: %w", kind, ref, err)
	}
	number, err := strconv.Atoi(strings.TrimSpace(numberText))
	if err != nil || number <= 0 {
		return fmt.Errorf("invalid %s reference %q: expected positive number", kind, ref)
	}
	return nil
}

func validateCollectionRepoRef(ref string) error {
	if strings.Count(ref, "/") != 1 {
		return fmt.Errorf("invalid repository reference %q", ref)
	}
	owner, repo, _ := strings.Cut(ref, "/")
	return (domain.RepoRef{Owner: strings.TrimSpace(owner), Repo: strings.TrimSpace(repo)}).Validate()
}

// ListCollections returns all named collections.
func (s *Service) ListCollections(ctx context.Context) (*cli.CollectionListResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
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
