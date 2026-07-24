package cli_test

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type fakeConcernService struct {
	*fakeService
	created contracts.ConcernCreateOptions
	listed  contracts.ConcernListOptions
	result  *contracts.ConcernResult
}

func (f *fakeConcernService) CreateConcern(_ context.Context, opts contracts.ConcernCreateOptions) (*contracts.ConcernResult, error) {
	f.created = opts
	return f.result, nil
}
func (f *fakeConcernService) ListConcerns(_ context.Context, opts contracts.ConcernListOptions) (*contracts.ConcernListResult, error) {
	f.listed = opts
	return &contracts.ConcernListResult{Concerns: []contracts.ConcernResult{*f.result}, Total: 1}, nil
}
func (f *fakeConcernService) ShowConcern(context.Context, string) (*contracts.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) UpdateConcern(context.Context, string, contracts.ConcernUpdateOptions) (*contracts.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) SetConcernStatus(context.Context, string, string, string) (*contracts.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) LinkConcern(context.Context, string, contracts.ConcernLinkOptions) (*contracts.ConcernResult, error) {
	return f.result, nil
}
func (f *fakeConcernService) PromoteConcern(context.Context, string, contracts.ConcernPromoteOptions) (*contracts.ConcernResult, error) {
	return f.result, nil
}

func TestConcernCreateAndSearchCLI(t *testing.T) {
	t.Parallel()
	svc := &fakeConcernService{fakeService: &fakeService{}, result: &contracts.ConcernResult{ID: "concern-1", Repo: contracts.RepoRef{Owner: "owner", Repo: "repo"}, Title: "flaky", Status: "untriaged"}}
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
