package contribution

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

type fakeRepo struct {
	issues map[string]*IssueDraft
	prs    map[string]*PullRequestDraft
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		issues: make(map[string]*IssueDraft),
		prs:    make(map[string]*PullRequestDraft),
	}
}

func (r *fakeRepo) SaveIssueDraft(_ context.Context, d *IssueDraft) error {
	r.issues[d.OpportunityID] = d
	return nil
}

func (r *fakeRepo) GetIssueDraft(_ context.Context, id string) (*IssueDraft, error) {
	return r.issues[id], nil
}

func (r *fakeRepo) SavePullRequestDraft(_ context.Context, d *PullRequestDraft) error {
	r.prs[d.OpportunityID] = d
	return nil
}

func (r *fakeRepo) GetPullRequestDraft(_ context.Context, id string) (*PullRequestDraft, error) {
	return r.prs[id], nil
}

func TestPrepareIssueStoresDraft(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	draft, err := svc.PrepareIssue(context.Background(), IssueInput{
		Opportunity: &investigation.Opportunity{
			ID:               "opp-1",
			Title:            "Fix race",
			ProblemStatement: "Race under load",
		},
		Evidence: []*evidence.Evidence{{ID: "ev-1", Type: evidence.EvidenceTypeManualObservation, Relation: evidence.RelationSupporting, Description: "observed"}},
		Guidance: "Add tests.",
		Success:  "Pass.",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if repo.issues["opp-1"] == nil {
		t.Fatal("draft was not persisted")
	}
	if draft.OpportunityID != "opp-1" {
		t.Fatalf("opportunity ID: got %q", draft.OpportunityID)
	}
}

func TestPreparePullRequestStoresDraft(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.PreparePullRequest(context.Background(), PullRequestInput{
		Opportunity: &investigation.Opportunity{
			ID:               "opp-1",
			Title:            "Fix race",
			ProblemStatement: "Race under load",
		},
		Approach: "Use mutex.",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if repo.prs["opp-1"] == nil {
		t.Fatal("PR draft was not persisted")
	}
}
