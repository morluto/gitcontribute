package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

type threadMetadataReader struct {
	repo   github.Repository
	issues []github.Issue
}

func (f *threadMetadataReader) GetRepository(ctx context.Context, owner, name string) (github.Repository, github.RateInfo, error) {
	return f.repo, github.RateInfo{}, nil
}

func (f *threadMetadataReader) ListIssues(ctx context.Context, owner, name string, opts github.ListIssueOptions) (github.ListResult[github.Issue], error) {
	return github.ListResult[github.Issue]{Items: f.issues, Page: github.PageInfo{}}, nil
}

func (*threadMetadataReader) GetRepositoryFile(_ context.Context, _, _, path string) (github.RepositoryFile, github.RateInfo, error) {
	return github.RepositoryFile{}, github.RateInfo{}, &github.NotFoundError{Resource: path}
}

func (f *threadMetadataReader) ListIssueComments(ctx context.Context, owner, name string, issueNumber int, opts github.PageOptions) (github.ListResult[github.IssueComment], error) {
	return github.ListResult[github.IssueComment]{}, nil
}

func (f *threadMetadataReader) GetPullRequestDetails(ctx context.Context, owner, name string, number int) (github.PullRequestDetails, github.RateInfo, error) {
	return github.PullRequestDetails{}, github.RateInfo{}, nil
}

func (f *threadMetadataReader) ListPullRequestReviews(ctx context.Context, owner, name string, number int, opts github.PageOptions) (github.ListResult[github.Review], error) {
	return github.ListResult[github.Review]{}, nil
}

func (f *threadMetadataReader) ListPullRequestComments(ctx context.Context, owner, name string, number int, opts github.PageOptions) (github.ListResult[github.ReviewComment], error) {
	return github.ListResult[github.ReviewComment]{}, nil
}

func TestSyncMapsIssueMetadataToThread(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	now := time.Unix(1000, 0).UTC()
	reader := &threadMetadataReader{
		repo: github.Repository{Owner: "owner", Name: "repo", NodeID: "R_1", UpdatedAt: now},
		issues: []github.Issue{{
			Number:            1,
			Kind:              github.ThreadKindIssue,
			State:             "closed",
			StateReason:       "completed",
			Title:             "bug",
			Body:              "details",
			Author:            "alice",
			AuthorAssociation: "OWNER",
			Labels:            []string{"bug"},
			Assignees:         []string{"charlie", "bob", "alice"},
			Draft:             false,
			Locked:            true,
			Milestone:         "v1.0",
			CreatedAt:         now,
			UpdatedAt:         now,
			ClosedAt:          &now,
		}},
	}
	svc.SetGitHubReader(reader)

	repoRef := contracts.RepoRef{Owner: "owner", Repo: "repo"}
	syncRepositoryContextForTest(t, svc, repoRef)
	if _, err := svc.ArchiveSync(ctx, repoRef, contracts.ArchiveSyncOptions{State: "all"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if repo == nil {
		t.Fatal("repository not found")
	}
	thread, err := c.GetThread(ctx, repo.ID, corpus.ThreadKindIssue, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread == nil {
		t.Fatal("thread not found")
	}
	if thread.StateReason != "completed" {
		t.Errorf("state_reason = %q, want completed", thread.StateReason)
	}
	if thread.AuthorAssociation != "OWNER" {
		t.Errorf("author_association = %q, want OWNER", thread.AuthorAssociation)
	}
	if thread.Milestone != "v1.0" {
		t.Errorf("milestone = %q, want v1.0", thread.Milestone)
	}
	if !thread.Locked {
		t.Errorf("locked = false, want true")
	}
	if thread.Draft {
		t.Errorf("draft = true, want false")
	}
	if len(thread.Assignees) != 3 || thread.Assignees[0] != "alice" || thread.Assignees[1] != "bob" || thread.Assignees[2] != "charlie" {
		t.Errorf("assignees = %v, want [alice bob charlie]", thread.Assignees)
	}
}
