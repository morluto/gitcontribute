package concern

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

type memoryRepository struct {
	items map[string]*Concern
	links map[string][]Link
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{items: map[string]*Concern{}, links: map[string][]Link{}}
}

func (r *memoryRepository) SaveConcern(_ context.Context, item *Concern) error {
	stored := cloneConcern(item)
	r.items[item.ID] = &stored
	return nil
}

func (r *memoryRepository) GetConcern(_ context.Context, id string) (*Concern, error) {
	item := r.items[id]
	if item == nil {
		return nil, ErrNotFound
	}
	stored := cloneConcern(item)
	stored.Links = append([]Link(nil), r.links[id]...)
	return &stored, nil
}

func (r *memoryRepository) ListConcerns(_ context.Context, _ Filter) ([]*Concern, error) {
	out := make([]*Concern, 0, len(r.items))
	for _, item := range r.items {
		stored := cloneConcern(item)
		out = append(out, &stored)
	}
	return out, nil
}

func (r *memoryRepository) AddConcernLink(_ context.Context, id string, link Link) error {
	if r.items[id] == nil {
		return ErrNotFound
	}
	r.links[id] = append(r.links[id], link)
	return nil
}

func TestConcernLifecycleAndLinks(t *testing.T) {
	t.Parallel()
	repo := newMemoryRepository()
	svc := NewService(repo)
	item, err := svc.Create(context.Background(), &Concern{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "abc",
		Title: " flaky test ", ProblemStatement: " fails intermittently ", Confidence: 0.4,
		Unknowns: []string{" timing ", "timing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusUntriaged || item.Title != "flaky test" || len(item.Unknowns) != 1 {
		t.Fatalf("unexpected concern: %+v", item)
	}
	if _, err := svc.SetStatus(context.Background(), item.ID, StatusInvestigating, "skip triage"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid transition error = %v", err)
	}
	if _, err := svc.SetStatus(context.Background(), item.ID, StatusAccepted, "worth checking"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetStatus(context.Background(), item.ID, StatusInvestigating, "begin proof"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(context.Background(), item.ID, Link{Kind: LinkDuplicateCandidate, TargetType: "thread", TargetID: "owner/repo:issue#7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetStatus(context.Background(), item.ID, StatusPromoted, "skip atomic promotion"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("direct promotion error = %v", err)
	}
	stored, err := svc.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.AuditTrail) != 2 || len(stored.Links) != 1 || stored.Status != StatusInvestigating {
		t.Fatalf("unexpected stored concern: %+v", stored)
	}
}

func TestConcernRejectsUnsafeOrUnboundedInput(t *testing.T) {
	t.Parallel()
	svc := NewService(newMemoryRepository())
	base := &Concern{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, CommitSHA: "abc", Title: "title", ProblemStatement: "problem"}
	badConfidence := *base
	badConfidence.Confidence = 2
	if _, err := svc.Create(context.Background(), &badConfidence); err == nil {
		t.Fatal("expected confidence error")
	}
	tooLarge := *base
	tooLarge.Notes = strings.Repeat("x", maxTextBytes+1)
	if _, err := svc.Create(context.Background(), &tooLarge); err == nil {
		t.Fatal("expected text bound error")
	}
	if _, err := svc.List(context.Background(), Filter{Limit: 101}); err == nil {
		t.Fatal("expected list bound error")
	}
}
