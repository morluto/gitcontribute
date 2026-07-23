package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/manifest"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestContributionManifestInvalidatesValidationWhenUntrackedContentChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	remote, _, candidateSHA := setupAppGitRemote(t)
	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, candidateSHA, "")
	if err != nil {
		t.Fatal(err)
	}
	hypothesis, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{Title: "bind workspace", Description: "validation must match candidate content", Category: investigation.CategoryBug})
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := svc.PromoteOpportunity(ctx, hypothesis.ID, "candidate proof can become stale", "workspace", "unsupported claims", "small", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.CreateWorkspace(ctx, inv.ID, cli.WorkspaceCreateOptions{Remote: remote, BaseRef: "master", CandidateRef: "feature", Name: "manifest"})
	if err != nil {
		t.Fatal(err)
	}
	writeAppFile(t, filepath.Join(ws.Path, "untracked.txt"), "first")
	manager, err := svc.workspaceReader()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.SnapshotByPath(ctx, ws.Path, ws.BaseSHA, ws.MergeBase)
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "repo-id", now, `{}`); err != nil {
		t.Fatal(err)
	}
	definition := &evidence.ValidationDefinition{ID: "def", InvestigationID: inv.ID, HypothesisID: hypothesis.ID, OpportunityID: opportunity.ID, Command: []string{"go", "test", "./..."}, WorkspaceID: ws.ID, CreatedAt: now}
	if err := c.SaveValidationDefinition(ctx, definition); err != nil {
		t.Fatal(err)
	}
	run := &evidence.ValidationRun{
		ID: "run", DefinitionID: definition.ID, InvestigationID: inv.ID, HypothesisID: hypothesis.ID, OpportunityID: opportunity.ID,
		Kind: evidence.RunKindCandidate, Classification: evidence.RunClassificationPassing, ObservationStatus: evidence.ObservationNotEvaluated,
		StartedAt: now, CompletedAt: now.Add(time.Minute), WorkspaceSnapshotBefore: snapshot.SHA256, WorkspaceSnapshotAfter: snapshot.SHA256,
		WorkspaceBindingStatus: "bound", WorkspaceBindingReason: "pre-run and post-run workspace identities match",
	}
	if err := c.SaveValidationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	older := *run
	older.ID = "run-old-failure"
	older.Classification = evidence.RunClassificationFailing
	older.StartedAt = now.Add(-2 * time.Minute)
	older.CompletedAt = now.Add(-time.Minute)
	if err := c.SaveValidationRun(ctx, &older); err != nil {
		t.Fatal(err)
	}
	first, err := svc.ContributionManifest(ctx, opportunity.ID, ManifestOptions{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	assertCurrentValidationSelected(t, first)
	assertFailingCandidateManifest(ctx, t, svc, c, opportunity.ID, ws.ID, run)
	writeAppFile(t, filepath.Join(ws.Path, "untracked.txt"), "second")
	second, err := svc.ContributionManifest(ctx, opportunity.ID, ManifestOptions{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.Predicate.Workspace.SHA256 == second.Predicate.Workspace.SHA256 {
		t.Fatal("untracked content change did not change workspace identity")
	}
	validation := second.Predicate.Validations[0]
	if validation.WorkspaceCompatibility != "stale" || !strings.Contains(validation.CompatibilityReason, "differs") {
		t.Fatalf("mutated workspace validation = %+v", validation)
	}
	if second.Predicate.Status != "incomplete" || !hasManifestGap(second.Predicate.Gaps, "validations") {
		t.Fatalf("manifest did not expose stale validation: status=%q gaps=%+v", second.Predicate.Status, second.Predicate.Gaps)
	}
	draft, err := svc.PrepareIssue(ctx, opportunity.ID, cli.PrepareIssueOptions{ManifestID: second.Predicate.ManifestID})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ManifestID != second.Predicate.ManifestID || strings.Contains(draft.Body, second.Predicate.ManifestID) {
		t.Fatalf("draft manifest reference = %q body=%q", draft.ManifestID, draft.Body)
	}
}

func assertCurrentValidationSelected(t *testing.T, statement *manifest.Statement) {
	t.Helper()
	if got := statement.Predicate.Validations[0].WorkspaceCompatibility; got != "compatible" {
		t.Fatalf("initial compatibility = %q", got)
	}
	if manifestHasGapCode(statement.Predicate.Gaps, "validation_outcome_failing") || selectedValidationCount(statement.Predicate.Validations) != 1 {
		t.Fatalf("superseded failure affected completeness: validations=%+v gaps=%+v", statement.Predicate.Validations, statement.Predicate.Gaps)
	}
}

func assertFailingCandidateManifest(ctx context.Context, t *testing.T, svc *Service, c *corpus.Corpus, opportunityID, workspaceID string, run *evidence.ValidationRun) {
	t.Helper()
	run.Classification = evidence.RunClassificationFailing
	if err := c.SaveValidationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	failed, err := svc.ContributionManifest(ctx, opportunityID, ManifestOptions{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if !manifestHasGapCode(failed.Predicate.Gaps, "validation_outcome_failing") {
		t.Fatalf("failing candidate validation was not exposed: %+v", failed.Predicate.Gaps)
	}
	run.Classification = evidence.RunClassificationPassing
	if err := c.SaveValidationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
}

func manifestHasGapCode(gaps []manifest.Gap, code string) bool {
	for _, gap := range gaps {
		if gap.Code == code {
			return true
		}
	}
	return false
}

func selectedValidationCount(records []manifest.ValidationRecord) int {
	selected := 0
	for _, record := range records {
		if record.Selected {
			selected++
		}
	}
	return selected
}

func TestContributionManifestKeepsMissingPullRequestFacetsIncomplete(t *testing.T) {
	fixture := newResearchFixture(t)
	started, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := fixture.svc.PromoteOpportunity(fixture.ctx, started.Hypothesis.ID, "stored PR health may be partial", "portfolio", "unsupported readiness", "small", 0.7)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := fixture.svc.ContributionManifest(fixture.ctx, opportunity.ID, ManifestOptions{
		PullRequest: &ManifestPullRequest{Owner: "owner", Repo: "repo", Number: 9},
	})
	if err != nil {
		t.Fatal(err)
	}
	if statement.Predicate.PullRequest == nil || statement.Predicate.Status != "incomplete" {
		t.Fatalf("partial PR manifest = %+v", statement.Predicate)
	}
	for _, facet := range statement.Predicate.PullRequest.Facets {
		if facet.Status != "complete" && !hasManifestGap(statement.Predicate.Gaps, "pull_request") {
			t.Fatalf("facet %q=%q was not represented as a gap", facet.Facet, facet.Status)
		}
	}
	if _, err := fixture.svc.ContributionManifest(fixture.ctx, opportunity.ID, ManifestOptions{
		PullRequest: &ManifestPullRequest{Owner: "other", Repo: "repo", Number: 9},
	}); err == nil || !strings.Contains(err.Error(), manifest.ErrIdentityMismatch.Error()) {
		t.Fatalf("repository mismatch error = %v", err)
	}
}
