package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/clusterprojection"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// ListClusters reads the current stored duplicate-candidate projection. It does
// not compute or write cluster state.
func (s *Service) ListClusters(ctx context.Context, repo contracts.RepoRef, limit int) (*contracts.ClusterListResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := validateClusterList(ref, limit); err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	projection, err := c.ListClusterProjection(ctx, ref, clustering.ClusterState(""), limit)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	return clusterListToCLI(repo, projection, limit), nil
}

// RefreshClusters explicitly computes and persists the duplicate-candidate
// projection for a repository.
func (s *Service) RefreshClusters(ctx context.Context, repo contracts.RepoRef) (*contracts.ClusterRefreshResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.DefaultComparisonBudget())
	if err != nil {
		return nil, err
	}
	snapshot, err := c.LoadClusterRefreshSnapshot(ctx, ref, engine.MaxCandidates())
	if err != nil {
		return nil, fmt.Errorf("load cluster refresh snapshot: %w", err)
	}
	if snapshot.CurrentProjection != nil && snapshot.CurrentProjection.Matches(snapshot.SourceRevision, snapshot.GovernanceRevision, engine.RuleVersion()) {
		return clusterRefreshToCLI(repo, "unchanged", *snapshot.CurrentProjection, clusterprojection.RefreshStats{
			CandidateCount: len(snapshot.Candidates), ClusterCount: currentProjectionClusterCount(snapshot.ExistingClusters), SnapshotQueries: snapshot.ReadStatements,
		}), nil
	}
	computation, err := engine.Cluster(ctx, snapshot.Candidates)
	if err != nil {
		return nil, fmt.Errorf("compute clusters: %w", err)
	}
	clusters, err := clustering.ReconcileProjection(ctx, computation.Clusters, snapshot.ExistingClusters, snapshot.OverridesByCluster, snapshot.Candidates, ref, snapshot.SourceRevision, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	stats := clusterprojection.RefreshStats{CandidateCount: computation.CandidateCount, PossiblePairs: computation.PossiblePairs, ScoredPairs: computation.ScoredPairs, ClusterCount: len(clusters), SnapshotQueries: snapshot.ReadStatements}
	committed, err := c.CommitClusterProjection(ctx, clusterprojection.Commit{Repo: ref, ExpectedSource: snapshot.SourceRevision, ExpectedGovernance: snapshot.GovernanceRevision, RuleVersion: engine.RuleVersion(), Clusters: clusters, Stats: stats, MaxCandidates: engine.MaxCandidates()})
	if err != nil {
		return nil, fmt.Errorf("commit cluster projection: %w", err)
	}
	disposition := "committed"
	if committed.Disposition == clusterprojection.AlreadyCurrent {
		disposition = "unchanged"
	}
	stats.CommitQueries = committed.WriteStatements
	return clusterRefreshToCLI(repo, disposition, committed.Projection, stats), nil
}

func currentProjectionClusterCount(clusters []clustering.Cluster) int {
	count := 0
	for _, cluster := range clusters {
		if cluster.State != clustering.ClusterRetired {
			count++
		}
	}
	return count
}

func clusterRefreshToCLI(repo contracts.RepoRef, disposition string, identity clusterprojection.Identity, stats clusterprojection.RefreshStats) *contracts.ClusterRefreshResult {
	return &contracts.ClusterRefreshResult{
		Repo: repo, Disposition: disposition,
		Projection: contracts.ClusterProjectionIdentity{SourceRevision: identity.SourceRevision, GovernanceRevision: identity.GovernanceRevision, RuleVersion: string(identity.RuleVersion), RunID: identity.RunID},
		Stats:      contracts.ClusterRefreshStats{CandidateCount: stats.CandidateCount, PossiblePairs: stats.PossiblePairs, ScoredPairs: stats.ScoredPairs, ClusterCount: stats.ClusterCount, SnapshotQueries: stats.SnapshotQueries, CommitQueries: stats.CommitQueries},
	}
}

func validateClusterList(ref domain.RepoRef, limit int) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if limit < 1 || limit > 1000 {
		return errors.New("cluster limit must be between 1 and 1000")
	}
	return nil
}

func clusterListToCLI(repo contracts.RepoRef, projection clusterprojection.List, limit int) *contracts.ClusterListResult {
	result := &contracts.ClusterListResult{
		Repo:      repo,
		Total:     projection.Total,
		Truncated: projection.Truncated,
	}
	if projection.Projection != nil {
		result.Projection = &contracts.ClusterProjectionIdentity{SourceRevision: projection.Projection.SourceRevision, GovernanceRevision: projection.Projection.GovernanceRevision, RuleVersion: string(projection.Projection.RuleVersion), RunID: projection.Projection.RunID}
	}
	for i, cl := range projection.Clusters {
		if i >= limit {
			break
		}
		result.Clusters = append(result.Clusters, *clusterToCLI(cl, 0))
	}
	return result
}

// Cluster returns a single cluster by stable id.
func (s *Service) Cluster(ctx context.Context, id string, limit int) (*contracts.ClusterResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("cluster id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, errors.New("cluster member limit cannot exceed 1000")
	}

	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	cl, err := c.GetClusterProjection(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if cl == nil {
		return nil, failure.NotFound(fmt.Errorf("cluster %q not found", id))
	}
	return clusterToCLI(*cl, limit), nil
}

func clusterToCLI(cl clustering.Cluster, memberLimit int) *contracts.ClusterResult {
	var members []contracts.ClusterMember
	if memberLimit > 0 {
		members = make([]contracts.ClusterMember, 0, len(cl.Members))
		for count, m := range cl.Members {
			if count >= memberLimit {
				break
			}
			members = append(members, contracts.ClusterMember{
				Kind:     m.Ref.Kind,
				Owner:    m.Ref.Owner,
				Repo:     m.Ref.Repo,
				Number:   m.Ref.Number,
				Title:    m.Title,
				State:    m.State,
				Score:    m.Score,
				Reason:   m.Reason,
				Included: m.Included,
			})
		}
	}
	return &contracts.ClusterResult{
		StableID:    cl.StableID,
		State:       string(cl.State),
		Canonical:   contracts.ClusterMember{Kind: cl.Canonical.Kind, Owner: cl.Canonical.Owner, Repo: cl.Canonical.Repo, Number: cl.Canonical.Number},
		MemberCount: len(cl.Members),
		Members:     members,
	}
}
