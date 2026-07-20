package cli_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type seedCLIService struct {
	*fakeService
	classes    []string
	polarities []string
	limit      int
}

func (s *seedCLIService) BuildDossierForCLI(context.Context, cli.RepoRef) (any, error) {
	return nil, s.err
}

func (s *seedCLIService) GetDossierForCLI(context.Context, cli.RepoRef) (any, error) {
	return nil, s.err
}

func (s *seedCLIService) ExtractSeedsForCLI(_ context.Context, _ cli.RepoRef, classes, polarities []string, limit int) (any, error) {
	s.classes = append([]string(nil), classes...)
	s.polarities = append([]string(nil), polarities...)
	s.limit = limit
	return []any{}, s.err
}

func TestSeedsCommandPassesExplicitPolarityControls(t *testing.T) {
	svc := &seedCLIService{fakeService: &fakeService{}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"seeds", "owner/repo", "--from=issues", "--polarity=negative,context", "--limit=7", "--json",
	}))

	if !reflect.DeepEqual(svc.classes, []string{"issues"}) {
		t.Fatalf("seed classes = %v", svc.classes)
	}
	if !reflect.DeepEqual(svc.polarities, []string{"negative", "context"}) {
		t.Fatalf("seed polarities = %v", svc.polarities)
	}
	if svc.limit != 7 {
		t.Fatalf("seed limit = %d", svc.limit)
	}
}

func TestSeedsCommandDefaultsToOutcomeEvidence(t *testing.T) {
	svc := &seedCLIService{fakeService: &fakeService{}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"seeds", "owner/repo", "--json"}))

	if !reflect.DeepEqual(svc.classes, []string{"merged-prs", "closed-prs", "issues"}) {
		t.Fatalf("default seed classes = %v", svc.classes)
	}
	if !reflect.DeepEqual(svc.polarities, []string{"positive", "negative"}) {
		t.Fatalf("default seed polarities = %v", svc.polarities)
	}
}
