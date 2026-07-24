package cli_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

type seedCLIService struct {
	*fakeService
	opts domain.ExtractSeedsOptions
	runs int
}

func (s *seedCLIService) BuildRepositoryDossier(context.Context, contracts.RepoRef) (*domain.Dossier, error) {
	return nil, s.err
}

func (s *seedCLIService) GetRepositoryDossier(context.Context, contracts.RepoRef) (*domain.Dossier, error) {
	return nil, s.err
}

func (s *seedCLIService) ExtractSeeds(_ context.Context, _ contracts.RepoRef, opts domain.ExtractSeedsOptions) ([]domain.Seed, error) {
	s.opts = opts
	s.runs++
	return nil, s.err
}

func TestSeedsCommandRejectsUnknownBoundaryValues(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"seeds", "owner/repo", "--from=invented"},
		{"seeds", "owner/repo", "--polarity=invented"},
	} {
		svc := &seedCLIService{fakeService: &fakeService{}}
		c, _, _ := newTestCLI(svc, nil)
		requireCLIError(t, c.Run(context.Background(), args), cli.ExitUsage)
		if svc.runs != 0 {
			t.Fatalf("ExtractSeeds called %d times for %v", svc.runs, args)
		}
	}
}

func TestSeedsCommandPassesExplicitPolarityControls(t *testing.T) {
	t.Parallel()
	svc := &seedCLIService{fakeService: &fakeService{}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"seeds", "owner/repo", "--from=issues", "--polarity=negative,context", "--limit=7", "--json",
	}))

	if !reflect.DeepEqual(svc.opts.Classes, []domain.SeedSourceClass{domain.SeedSourceClassIssue}) {
		t.Fatalf("seed classes = %v", svc.opts.Classes)
	}
	if !reflect.DeepEqual(svc.opts.Polarities, []domain.SeedPolarity{domain.SeedPolarityNegative, domain.SeedPolarityContext}) {
		t.Fatalf("seed polarities = %v", svc.opts.Polarities)
	}
	if svc.opts.Limit != 7 {
		t.Fatalf("seed limit = %d", svc.opts.Limit)
	}
}

func TestSeedsCommandDefaultsToOutcomeEvidence(t *testing.T) {
	t.Parallel()
	svc := &seedCLIService{fakeService: &fakeService{}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"seeds", "owner/repo", "--json"}))

	wantClasses := []domain.SeedSourceClass{
		domain.SeedSourceClassMergedPR,
		domain.SeedSourceClassClosedUnmergedPR,
		domain.SeedSourceClassIssue,
	}
	if !reflect.DeepEqual(svc.opts.Classes, wantClasses) {
		t.Fatalf("default seed classes = %v", svc.opts.Classes)
	}
	wantPolarities := []domain.SeedPolarity{domain.SeedPolarityPositive, domain.SeedPolarityNegative}
	if !reflect.DeepEqual(svc.opts.Polarities, wantPolarities) {
		t.Fatalf("default seed polarities = %v", svc.opts.Polarities)
	}
}
