package corpus

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestStartThreadInvestigationIsAtomicAndOpenIdempotent(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	firstInvestigation, firstHypothesis := threadInvestigationPair("inv-1", "hyp-1", 1, now)
	storedInvestigation, storedHypothesis, created, err := c.StartThreadInvestigation(ctx, firstInvestigation, firstHypothesis)
	if err != nil {
		t.Fatal(err)
	}
	if !created || storedInvestigation.ID != "inv-1" || storedHypothesis.ID != "hyp-1" {
		t.Fatalf("first pair = (%+v, %+v, %t)", storedInvestigation, storedHypothesis, created)
	}

	secondInvestigation, secondHypothesis := threadInvestigationPair("inv-2", "hyp-2", 1, now.Add(time.Hour))
	storedInvestigation, storedHypothesis, created, err = c.StartThreadInvestigation(ctx, secondInvestigation, secondHypothesis)
	if err != nil {
		t.Fatal(err)
	}
	if created || storedInvestigation.ID != "inv-1" || storedHypothesis.ID != "hyp-1" || storedInvestigation.ThreadBaseline.ObservationID != 1 {
		t.Fatalf("idempotent pair = (%+v, %+v, %t)", storedInvestigation, storedHypothesis, created)
	}

	storedInvestigation.Status = investigation.InvestigationClosed
	storedInvestigation.UpdatedAt = now.Add(2 * time.Hour)
	if err := c.SaveInvestigation(ctx, storedInvestigation); err != nil {
		t.Fatal(err)
	}
	thirdInvestigation, thirdHypothesis := threadInvestigationPair("inv-3", "hyp-3", 2, now.Add(3*time.Hour))
	_, _, created, err = c.StartThreadInvestigation(ctx, thirdInvestigation, thirdHypothesis)
	if err != nil || !created {
		t.Fatalf("start after close = created:%t err:%v", created, err)
	}
}

func TestStartThreadInvestigationRollsBackLateHypothesisFailure(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	existingInvestigation := &investigation.Investigation{
		ID: "existing", Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Status: investigation.InvestigationOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := c.SaveInvestigation(ctx, existingInvestigation); err != nil {
		t.Fatal(err)
	}
	existingHypothesis := &investigation.Hypothesis{
		ID: "hyp-conflict", InvestigationID: existingInvestigation.ID, Title: "existing",
		Category: investigation.CategoryOther, Status: investigation.HypothesisProposed, CreatedAt: now, UpdatedAt: now,
	}
	if err := c.SaveHypothesis(ctx, existingHypothesis); err != nil {
		t.Fatal(err)
	}

	candidateInvestigation, candidateHypothesis := threadInvestigationPair("candidate", "hyp-conflict", 9, now.Add(time.Hour))
	_, _, _, err := c.StartThreadInvestigation(ctx, candidateInvestigation, candidateHypothesis)
	if err == nil {
		t.Fatal("expected seed hypothesis conflict")
	}
	if stored, getErr := c.GetInvestigation(ctx, candidateInvestigation.ID); !errors.Is(getErr, investigation.ErrNotFound) || stored != nil {
		t.Fatalf("late failure left investigation = (%+v, %v)", stored, getErr)
	}
}

func TestStartThreadInvestigationConcurrentRequestsShareOpenPair(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	type outcome struct {
		investigationID string
		hypothesisID    string
		created         bool
		err             error
	}
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for index := 1; index <= 2; index++ {
		suffix := strconv.Itoa(index)
		item, hypothesis := threadInvestigationPair(
			"concurrent-inv-"+suffix, "concurrent-hyp-"+suffix, int64(index), now,
		)
		wait.Add(1)
		go func() {
			defer wait.Done()
			storedInvestigation, storedHypothesis, created, err := c.StartThreadInvestigation(ctx, item, hypothesis)
			result := outcome{created: created, err: err}
			if storedInvestigation != nil {
				result.investigationID = storedInvestigation.ID
			}
			if storedHypothesis != nil {
				result.hypothesisID = storedHypothesis.ID
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	createdCount := 0
	var investigationID, hypothesisID string
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			createdCount++
		}
		if investigationID == "" {
			investigationID, hypothesisID = result.investigationID, result.hypothesisID
		}
		if result.investigationID != investigationID || result.hypothesisID != hypothesisID {
			t.Fatalf("concurrent results disagree: got (%s, %s), want (%s, %s)", result.investigationID, result.hypothesisID, investigationID, hypothesisID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestStartThreadInvestigationRejectsRepositoryMismatch(t *testing.T) {
	c, _ := openTestCorpus(t)
	item, hypothesis := threadInvestigationPair("inv", "hyp", 1, time.Now().UTC())
	item.Repo = domain.RepoRef{Owner: "other", Repo: "repo"}
	_, _, _, err := c.StartThreadInvestigation(context.Background(), item, hypothesis)
	if !errors.Is(err, investigation.ErrInvalidThreadBaseline) {
		t.Fatalf("repository mismatch error = %v", err)
	}
}

func threadInvestigationPair(investigationID, hypothesisID string, observationID int64, now time.Time) (*investigation.Investigation, *investigation.Hypothesis) {
	baseline := &investigation.ThreadBaseline{
		Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: domain.IssueKind, Number: 1,
		ObservationID: observationID, ObservationSequence: observationID,
		SourceUpdatedAt: now, ObservedAt: now,
		Source: domain.SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/o/r/issues/1", ObservedAt: now, AsOf: now},
	}
	item := &investigation.Investigation{
		ID: investigationID, Repo: baseline.Repo, Status: investigation.InvestigationOpen,
		ThreadBaseline: baseline, SeedHypothesisID: hypothesisID, CreatedAt: now, UpdatedAt: now,
	}
	hypothesis := &investigation.Hypothesis{
		ID: hypothesisID, InvestigationID: investigationID, Title: "title",
		Category: investigation.CategoryOther, Status: investigation.HypothesisProposed,
		SourceRefs: []domain.SourceRef{baseline.Source}, CreatedAt: now, UpdatedAt: now,
	}
	return item, hypothesis
}
