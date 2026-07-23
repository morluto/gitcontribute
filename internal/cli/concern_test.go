package cli_test

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type fakeConcernService struct {
	*fakeService
	created cli.ConcernCreateOptions
	listed  cli.ConcernListOptions
	result  *cli.ConcernResult
}

func (f *fakeConcernService) CreateConcern(_ context.Context, opts cli.ConcernCreateOptions) (*cli.ConcernResult, error) {
	f.created = opts
	return f.result, nil
}
func (f *fakeConcernService) ListConcerns(_ context.Context, opts cli.ConcernListOptions) (*cli.ConcernListResult, error) {
	f.listed = opts
	return &cli.ConcernListResult{Concerns: []cli.ConcernResult{*f.result}, Total: 1}, nil
}
func (f *fakeConcernService) ShowConcern(context.Context, string) (*cli.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) UpdateConcern(context.Context, string, cli.ConcernUpdateOptions) (*cli.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) SetConcernStatus(context.Context, string, string, string) (*cli.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) LinkConcern(context.Context, string, cli.ConcernLinkOptions) (*cli.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) PromoteConcern(context.Context, string, cli.ConcernPromoteOptions) (*cli.ConcernResult, error) {
	return f.result, nil
}

func TestConcernCreateAndSearchCLI(t *testing.T) {
	svc := &fakeConcernService{fakeService: &fakeService{}, result: &cli.ConcernResult{ID: "concern-1", Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, Title: "flaky", Status: "untriaged"}}
	c, _, _ := newTestCLI(svc, nil)
	if err := c.Run(context.Background(), []string{"concern", "create", "owner/repo", "--commit", "abc", "--title", "flaky", "--problem", "intermittent", "--unknown", "timing"}); err != nil {
		t.Fatal(err)
	}
	if svc.created.Repo.Owner != "owner" || svc.created.CommitSHA != "abc" || len(svc.created.Unknowns) != 1 {
		t.Fatalf("create options = %+v", svc.created)
	}
	if err := c.Run(context.Background(), []string{"concern", "list", "owner/repo", "--query", "timing", "--limit", "5"}); err != nil {
		t.Fatal(err)
	}
	if svc.listed.Query != "timing" || svc.listed.Limit != 5 {
		t.Fatalf("list options = %+v", svc.listed)
	}
}
