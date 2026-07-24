package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/radar"
)

type radarFakeService struct {
	*fakeService
	called bool
	result *radar.Report
	opts   cli.RadarOptions
}

func (f *radarFakeService) ContributionRadar(_ context.Context, opts cli.RadarOptions) (*radar.Report, error) {
	f.called = true
	f.opts = opts
	return f.result, f.err
}

func TestRadarRendersExplanations(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	svc := &radarFakeService{fakeService: &fakeService{}, result: &radar.Report{
		Repo: "o/r", ScoreVersion: radar.ScoreVersion, SourceAsOf: now, TotalOpenIssues: 3, CandidatePopulation: 3,
		Candidates: []radar.Candidate{{
			Rank: 1, Number: 42, Title: "Fix flaky retry", URL: "https://github.com/o/r/issues/42",
			Eligibility: radar.EligibilityReadyToCode, Score: 82, Confidence: "high",
			PositiveSignals: []radar.Signal{{Code: "help_wanted", Summary: "maintainers marked this as open to outside help", Weight: 12}},
			Risks:           []radar.Signal{{Code: "aging_issue", Summary: "issue has aged", Weight: -5}},
			RelatedWork: []radar.RelatedWork{{
				Ref: "pull_request:o/r#9", Relation: "depends_on", Direction: "outbound", State: "open", Evidence: []radar.RelatedWorkEvidence{},
			}},
		}},
		Unknowns: []radar.Unknown{{Code: "contribution_guidance_unknown", Summary: "contribution guidance is unknown"}},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	requireNoErr(t, c.Run(context.Background(), []string{"radar", "o/r"}))
	if !svc.called || svc.opts.Repo.String() != "o/r" || svc.opts.Limit != 20 {
		t.Fatalf("radar call = called:%v opts:%+v", svc.called, svc.opts)
	}
	for _, want := range []string{"Contribution radar: o/r (radar.v3)", "[ready_to_code] #42", "why:", "risks:", "related: depends_on pull_request:o/r#9 [open]", "Repository unknowns:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, stdout.String())
		}
	}
}

func TestRadarJSONAndLimitValidation(t *testing.T) {
	t.Parallel()
	svc := &radarFakeService{fakeService: &fakeService{}, result: &radar.Report{Repo: "o/r", ScoreVersion: radar.ScoreVersion, Candidates: []radar.Candidate{}}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"radar", "o/r", "--limit", "500", "--json"}))
	var got radar.Report
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid radar JSON: %v\n%s", err, stdout.String())
	}
	if got.Repo != "o/r" || svc.opts.Limit != radar.MaxLimit {
		t.Fatalf("result=%+v opts=%+v", got, svc.opts)
	}

	svc.called = false
	if err := c.Run(context.Background(), []string{"radar", "o/r", "--limit", "0"}); err == nil {
		t.Fatal("expected invalid limit error")
	} else {
		requireCLIError(t, err, cli.ExitUsage)
	}
	if svc.called {
		t.Fatal("radar should not run with an invalid limit")
	}
	for _, args := range [][]string{
		{"radar"},
		{"radar", "--repo", "o/r"},
		{"radar", "o/r", "--limit", "501"},
	} {
		if err := c.Run(context.Background(), args); err == nil {
			t.Fatalf("expected usage error for %v", args)
		} else {
			requireCLIError(t, err, cli.ExitUsage)
		}
	}
}
