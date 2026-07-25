package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/workspace"
)

func TestWorkspaceDiffAndReviewReport(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	remote, baseSHA, candidateSHA := setupAppGitRemote(t)

	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, candidateSHA, "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	ws, err := svc.CreateWorkspace(ctx, inv.ID, contracts.WorkspaceCreateOptions{
		Remote:       remote,
		BaseRef:      "master",
		CandidateRef: "feature",
		Name:         "ws-review",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(ws.Path, "base.txt")); err != nil {
		t.Fatalf("delete tracked workspace file: %v", err)
	}

	diff, err := svc.WorkspaceDiff(ctx, ws.ID)
	if err != nil {
		t.Fatalf("workspace diff: %v", err)
	}
	if diff.BaseSHA != baseSHA || diff.CandidateSHA != candidateSHA {
		t.Fatalf("unexpected diff metadata: %+v", diff)
	}
	if len(diff.ChangedFiles) == 0 {
		t.Fatalf("expected changed files")
	}
	if !slices.Contains(diff.ChangedFiles, "base.txt") {
		t.Fatalf("deleted file missing from changed files: %v", diff.ChangedFiles)
	}
	if len(diff.ReviewOrder) == 0 {
		t.Fatalf("expected review order")
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}

	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{
		Title: "race", Description: "desc", Category: investigation.CategoryBug,
	})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}
	o, err := svc.PromoteOpportunityWithInput(ctx, h.ID, investigation.PromoteOpportunityInput{
		ProblemStatement: "missing feature",
		Scope:            "pkg/feature",
		Impact:           "improvement",
		ExpectedEffort:   "small",
		Confidence:       0.7,
	})
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}

	report, err := svc.PrepareReviewReport(ctx, contracts.PrepareReviewReportInput{
		OpportunityID: o.ID,
		WorkspaceID:   ws.ID,
	})
	if err != nil {
		t.Fatalf("prepare review report: %v", err)
	}
	if report.DiffMetadata == nil || len(report.DiffMetadata.ChangedFiles) == 0 {
		t.Fatalf("review report missing diff metadata: %+v", report)
	}
	if len(report.SuggestedReviewOrder) == 0 {
		t.Fatalf("review report missing suggested review order")
	}

	// Wrong-workspace rejection: an unrelated workspace cannot be attached to the opportunity.
	if err := c.SaveWorkspace(ctx, &workspace.Workspace{
		Name:            "unrelated-workspace",
		InvestigationID: "another-investigation",
		RepoOwner:       "other",
		RepoName:        "repo",
		Path:            t.TempDir(),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.PrepareReviewReport(ctx, contracts.PrepareReviewReportInput{
		OpportunityID: o.ID,
		WorkspaceID:   "unrelated-workspace",
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected wrong-workspace rejection, got %v", err)
	}
}
