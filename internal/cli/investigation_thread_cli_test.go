package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/research"
)

type threadInvestigationFakeService struct {
	*fakeService
	called bool
	ref    research.ThreadRef
	result *contracts.ThreadInvestigationResult
}

func (f *threadInvestigationFakeService) StartInvestigationFromThread(_ context.Context, ref research.ThreadRef) (*contracts.ThreadInvestigationResult, error) {
	f.called = true
	f.ref = ref
	return f.result, f.err
}

func TestInvestigationStartThreadParsesTypedRefAndRendersJSON(t *testing.T) {
	t.Parallel()
	result := &contracts.ThreadInvestigationResult{
		Created: true,
		Investigation: &contracts.InvestigationResult{
			ID: "inv", Repo: contracts.RepoRef{Owner: "o", Repo: "r"}, Status: "open",
			ThreadBaseline: &contracts.ThreadBaselineResult{Ref: "pull_request:o/r#7", ObservationID: 11, ObservationSequence: 4},
		},
		Hypothesis: &contracts.HypothesisResult{ID: "hyp", InvestigationID: "inv", Title: "stored title", Category: "other", Status: "proposed"},
	}
	service := &threadInvestigationFakeService{fakeService: &fakeService{}, result: result}
	c, stdout, _ := newTestCLI(service, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"investigation", "start-thread", "pr:o/r#7", "--json"}))
	if !service.called || service.ref.Kind != domain.PullRequestKind || service.ref.Repo.String() != "o/r" || service.ref.Number != 7 {
		t.Fatalf("start-thread call = called:%t ref:%+v", service.called, service.ref)
	}
	var output contracts.ThreadInvestigationResult
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if !output.Created || output.Investigation.ThreadBaseline.ObservationID != 11 || output.Hypothesis.ID != "hyp" {
		t.Fatalf("output = %+v", output)
	}
}

func TestInvestigationStartThreadValidatesRefAndCapability(t *testing.T) {
	t.Parallel()
	service := &threadInvestigationFakeService{fakeService: &fakeService{}}
	c, _, _ := newTestCLI(service, nil)
	err := c.Run(context.Background(), []string{"investigation", "start-thread", "bad-ref"})
	if err == nil {
		t.Fatal("expected invalid reference error")
	}
	requireCLIError(t, err, cli.ExitUsage)
	if service.called {
		t.Fatal("invalid reference reached service")
	}

	c, _, _ = newTestCLI(&fakeService{}, nil)
	err = c.Run(context.Background(), []string{"investigation", "start-thread", "o/r#1"})
	if err == nil {
		t.Fatal("expected not-wired error")
	}
	requireCLIError(t, err, cli.ExitNotWired)
}

func TestInvestigationStartThreadHumanOutputExplainsReuse(t *testing.T) {
	t.Parallel()
	result := &contracts.ThreadInvestigationResult{
		Created: false,
		Investigation: &contracts.InvestigationResult{
			ID: "inv", Repo: contracts.RepoRef{Owner: "o", Repo: "r"}, Status: "open",
			ThreadBaseline: &contracts.ThreadBaselineResult{
				Ref: "issue:o/r#1", ObservationID: 8, ObservationSequence: 3, SourceUpdatedAt: "2026-07-17T12:00:00Z",
			},
		},
		Hypothesis: &contracts.HypothesisResult{ID: "hyp", InvestigationID: "inv", Title: "title", Category: "other", Status: "proposed"},
	}
	service := &threadInvestigationFakeService{fakeService: &fakeService{}, result: result}
	c, stdout, _ := newTestCLI(service, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"investigation", "start-thread", "o/r#1"}))
	for _, want := range []string{"Reused open investigation inv", "issue:o/r#1 observation 8", "sequence 3"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("human output missing %q: %s", want, stdout.String())
		}
	}
}
