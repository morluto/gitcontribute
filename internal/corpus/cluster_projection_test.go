package corpus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/clusterprojection"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestCommitClusterProjectionRejectsChangedSource(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread := Thread{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 1, State: "open", Title: "first", SourceUpdatedAt: time.Unix(1, 0).UTC()}
	if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "rocket"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}
	thread.Title = "changed"
	thread.SourceUpdatedAt = time.Unix(2, 0).UTC()
	if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
		t.Fatal(err)
	}
	_, err = c.CommitClusterProjection(ctx, clusterprojection.Commit{Repo: ref, ExpectedSource: snapshot.SourceRevision, RuleVersion: similarity.DuplicateV1, MaxCandidates: maxCandidates})
	var stale *clusterprojection.StaleInputError
	if !errors.As(err, &stale) || stale.ActualSource == stale.ExpectedSource {
		t.Fatalf("commit error = %v, want changed-source StaleInputError", err)
	}
	after, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentProjection != nil {
		t.Fatalf("stale commit advanced projection: %+v", after.CurrentProjection)
	}
}

func TestConcurrentIdenticalEmptyProjectionHasOneCurrentRun(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "empty"}, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "empty"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}
	commit := clusterprojection.Commit{Repo: ref, ExpectedSource: snapshot.SourceRevision, ExpectedGovernance: snapshot.GovernanceRevision, RuleVersion: similarity.DuplicateV1, MaxCandidates: maxCandidates}
	results := make(chan clusterprojection.CommitDisposition, 2)
	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			result, err := c.CommitClusterProjection(ctx, commit)
			if err != nil {
				errorsOut <- err
				return
			}
			results <- result.Disposition
		}()
	}
	seen := map[clusterprojection.CommitDisposition]int{}
	for range 2 {
		select {
		case err := <-errorsOut:
			t.Fatal(err)
		case disposition := <-results:
			seen[disposition]++
		}
	}
	if seen[clusterprojection.Committed] != 1 || seen[clusterprojection.AlreadyCurrent] != 1 {
		t.Fatalf("dispositions = %+v", seen)
	}
	after, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentProjection == nil || after.CurrentProjection.RunID == 0 {
		t.Fatalf("empty projection identity = %+v", after.CurrentProjection)
	}
	listed, err := c.ListClusterProjection(ctx, ref, "", 10)
	if err != nil || len(listed.Clusters) != 0 || listed.Projection == nil {
		t.Fatalf("empty projection list = %+v, err=%v", listed, err)
	}
}

func TestCommitClusterProjectionRejectsMissingRuleVersion(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "empty"}, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "empty"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:               ref,
		ExpectedSource:     snapshot.SourceRevision,
		ExpectedGovernance: snapshot.GovernanceRevision,
		MaxCandidates:      maxCandidates,
	})
	if err == nil || err.Error() != "cluster rule version is required" {
		t.Fatalf("commit error = %v, want missing rule version", err)
	}

	after, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentProjection != nil {
		t.Fatalf("invalid commit advanced projection: %+v", after.CurrentProjection)
	}
}

func TestCommitClusterProjectionRejectsMissingSourceRevision(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "acme", Repo: "rocket"}

	_, err := c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:          ref,
		RuleVersion:   similarity.DuplicateV1,
		MaxCandidates: clustering.DefaultExactPairBudget().MaxCandidates(),
	})
	if err == nil || err.Error() != "cluster source revision is required" {
		t.Fatalf("commit error = %v, want missing source revision", err)
	}
}

func TestCommitClusterProjectionRejectsInvalidCandidateBound(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	_, err := c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:           domain.RepoRef{Owner: "acme", Repo: "rocket"},
		ExpectedSource: "revision",
		RuleVersion:    similarity.DuplicateV1,
	})
	if err == nil || err.Error() != "max candidates must be positive" {
		t.Fatalf("commit error = %v, want invalid candidate bound", err)
	}
}

func TestCommitClusterProjectionRejectsInvalidRepository(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	_, err := c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:           domain.RepoRef{Owner: "acme"},
		ExpectedSource: "revision",
		RuleVersion:    similarity.DuplicateV1,
		MaxCandidates:  clustering.DefaultExactPairBudget().MaxCandidates(),
	})
	if err == nil || err.Error() != "repo is required" {
		t.Fatalf("commit error = %v, want invalid repository", err)
	}
}

func TestCommitClusterProjectionRejectsClusterFromDifferentSource(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket"}, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "rocket"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:               ref,
		ExpectedSource:     snapshot.SourceRevision,
		ExpectedGovernance: snapshot.GovernanceRevision,
		RuleVersion:        similarity.DuplicateV1,
		MaxCandidates:      maxCandidates,
		Clusters: []clustering.Cluster{{
			StableID: "cluster-1",
			Repo:     ref,
			Revision: "different-source",
			State:    clustering.ClusterOpen,
			Canonical: clustering.MemberRef{
				Owner: "acme", Repo: "rocket", Kind: ThreadKindIssue, Number: 1,
			},
		}},
	})
	if err == nil || err.Error() != `cluster "cluster-1" source revision does not match commit` {
		t.Fatalf("commit error = %v, want cluster source mismatch", err)
	}
}

func TestCommitClusterProjectionRejectsClusterWithoutStableIdentity(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket"}, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "rocket"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:               ref,
		ExpectedSource:     snapshot.SourceRevision,
		ExpectedGovernance: snapshot.GovernanceRevision,
		RuleVersion:        similarity.DuplicateV1,
		MaxCandidates:      maxCandidates,
		Clusters:           []clustering.Cluster{{Repo: ref, Revision: snapshot.SourceRevision}},
	})
	if err == nil || err.Error() != "cluster stable id is required" {
		t.Fatalf("commit error = %v, want missing stable identity", err)
	}
}

func TestCommitClusterProjectionRejectsClusterFromDifferentRepository(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket"}, `{}`); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepoRef{Owner: "acme", Repo: "rocket"}
	maxCandidates := clustering.DefaultExactPairBudget().MaxCandidates()
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, maxCandidates)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.CommitClusterProjection(ctx, clusterprojection.Commit{
		Repo:               ref,
		ExpectedSource:     snapshot.SourceRevision,
		ExpectedGovernance: snapshot.GovernanceRevision,
		RuleVersion:        similarity.DuplicateV1,
		MaxCandidates:      maxCandidates,
		Clusters: []clustering.Cluster{{
			StableID: "cluster-1",
			Repo:     domain.RepoRef{Owner: "other", Repo: "repo"},
			Revision: snapshot.SourceRevision,
		}},
	})
	if err == nil || err.Error() != `cluster "cluster-1" repository does not match commit` {
		t.Fatalf("commit error = %v, want cluster repository mismatch", err)
	}
}
