package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

// CreateCollection creates a named collection.
func (s *Service) CreateCollection(ctx context.Context, name string) (*contracts.CollectionResult, error) {
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
func (s *Service) AddCollectionMembers(ctx context.Context, name string, members []contracts.CollectionMember) (*contracts.CollectionResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("collection name is required")
	}
	if len(members) == 0 {
		return nil, errors.New("at least one member is required")
	}

	cm := make([]corpus.CollectionMember, len(members))
	for i, m := range members {
		parsed, err := parseCollectionMember(m)
		if err != nil {
			return nil, fmt.Errorf("member %d: %w", i+1, err)
		}
		cm[i] = parsed
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
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

func parseCollectionMember(member contracts.CollectionMember) (corpus.CollectionMember, error) {
	kind := strings.TrimSpace(member.Kind)
	ref := strings.TrimSpace(member.Ref)
	if ref == "" {
		return corpus.CollectionMember{}, errors.New("collection member reference is required")
	}
	switch kind {
	case "repository":
		parsed, err := domain.ParseRepoRef(ref)
		if err != nil {
			return corpus.CollectionMember{}, err
		}
		return corpus.NewRepositoryCollectionMember(parsed)
	case "issue", "pull_request", "thread":
		repository, number, err := parseCollectionThreadRef(kind, ref)
		if err != nil {
			return corpus.CollectionMember{}, err
		}
		if kind == "thread" {
			return corpus.NewAnyThreadCollectionMember(repository, number)
		}
		threadKind, err := domain.ParseThreadKind(kind)
		if err != nil {
			return corpus.CollectionMember{}, err
		}
		return corpus.NewThreadCollectionMember(threadKind, repository, number)
	case "opportunity", "investigation":
		if len(ref) > 64 {
			return corpus.CollectionMember{}, fmt.Errorf("invalid %s reference %q: exceeds 64 bytes", kind, ref)
		}
		id, err := uuid.Parse(ref)
		if err != nil {
			return corpus.CollectionMember{}, fmt.Errorf("invalid %s reference %q: expected durable id", kind, ref)
		}
		if kind == "opportunity" {
			return corpus.NewOpportunityCollectionMember(id.String())
		}
		return corpus.NewInvestigationCollectionMember(id.String())
	default:
		return corpus.CollectionMember{}, fmt.Errorf("unsupported collection member kind %q", kind)
	}
}

func parseCollectionThreadRef(kind, ref string) (domain.RepoRef, int, error) {
	if strings.Count(ref, "#") != 1 {
		return domain.RepoRef{}, 0, fmt.Errorf("invalid %s reference %q: expected OWNER/REPO#NUMBER", kind, ref)
	}
	repoRef, numberText, _ := strings.Cut(ref, "#")
	parsedRepo, err := domain.ParseRepoRef(repoRef)
	if err != nil {
		return domain.RepoRef{}, 0, fmt.Errorf("invalid %s reference %q: %w", kind, ref, err)
	}
	number, err := strconv.Atoi(strings.TrimSpace(numberText))
	if err != nil || number <= 0 {
		return domain.RepoRef{}, 0, fmt.Errorf("invalid %s reference %q: expected positive number", kind, ref)
	}
	return parsedRepo, number, nil
}

// ListCollections returns all named collections.
func (s *Service) ListCollections(ctx context.Context) (*contracts.CollectionListResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	list, err := c.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	result := &contracts.CollectionListResult{
		Collections: make([]contracts.CollectionResult, len(list.Collections)),
		Total:       list.Total,
		Truncated:   list.Truncated,
	}
	for i, col := range list.Collections {
		result.Collections[i] = *collectionResult(&col)
	}
	return result, nil
}

func collectionResult(c *corpus.Collection) *contracts.CollectionResult {
	return &contracts.CollectionResult{
		Name:        c.Name,
		MemberCount: c.MemberCount,
		CreatedAt:   formatTime(c.CreatedAt),
		UpdatedAt:   formatTime(c.UpdatedAt),
	}
}
