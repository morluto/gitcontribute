package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
)

func TestPublicCorpusReadsDoNotCreateDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name string
		read func(context.Context, *Service) error
	}{
		{
			name: "show investigation",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ShowInvestigation(ctx, "missing")
				return err
			},
		},
		{
			name: "list investigations",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ListInvestigations(ctx)
				return err
			},
		},
		{
			name: "list hypotheses",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ListHypotheses(ctx, "missing")
				return err
			},
		},
		{
			name: "show opportunity",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ShowOpportunity(ctx, "missing")
				return err
			},
		},
		{
			name: "list opportunities",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ListOpportunities(ctx, "")
				return err
			},
		},
		{
			name: "show evidence",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ShowEvidence(ctx, "missing")
				return err
			},
		},
		{
			name: "show workspace",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ShowWorkspace(ctx, "missing")
				return err
			},
		},
		{
			name: "workspace diff",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.WorkspaceDiff(ctx, "missing")
				return err
			},
		},
		{
			name: "list jobs",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.ListJobs(ctx, "", 10)
				return err
			},
		},
		{
			name: "get job",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.GetJob(ctx, "missing")
				return err
			},
		},
		{
			name: "prepare review",
			read: func(ctx context.Context, svc *Service) error {
				_, err := svc.PrepareReviewReport(ctx, PrepareReviewReportInput{WorkspaceID: "missing"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(config.NewPaths(&config.Env{Home: t.TempDir()}), "test", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			database := svc.databasePath()
			if _, err := os.Stat(database); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("database exists before read: %v", err)
			}
			if err := tt.read(ctx, svc); err == nil {
				t.Fatal("read unexpectedly succeeded without a corpus")
			}
			if _, err := os.Stat(database); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read created database: %v", err)
			}
			if svc.corpus != nil {
				t.Fatal("read initialized writable corpus")
			}
			if svc.jobs != nil {
				t.Fatal("read initialized job executor")
			}
		})
	}
}

func TestInvestigationWriteStillInitializesCorpus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, err := New(config.NewPaths(&config.Env{Home: t.TempDir()}), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	if _, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.databasePath()); err != nil {
		t.Fatalf("database after write: %v", err)
	}
}
