package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// FindClusters lists duplicate-candidate clusters for bounded repository
// targets from the local corpus without recomputing them.
func (r *MCPReader) FindClusters(ctx context.Context, in mcpcontract.FindClustersInput) (mcpcontract.FindClustersOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 20 {
		return mcpcontract.FindClustersOutput{}, mcpcontract.InvalidArgument("targets", "must contain 1 to 20 items", map[string]any{
			"targets": []map[string]string{{"owner": "acme", "repo": "rocket"}},
		})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit <= 0 || in.Limit > 100 {
		return mcpcontract.FindClustersOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.FindClustersOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.FindClustersOutput{}, err
	}
	out := mcpcontract.FindClustersOutput{
		Status:        "complete",
		Items:         make([]mcpcontract.BatchItem[mcpcontract.ClusterSetOutput], len(in.Targets)),
		SnapshotToken: snapshotIdentity(in.SnapshotToken, revision),
	}
	seen := make(map[string]struct{}, len(in.Targets))
	for i, target := range in.Targets {
		key := clusterTargetKey(target)
		item := mcpcontract.BatchItem[mcpcontract.ClusterSetOutput]{Key: key, Status: "complete"}
		if err := validateClusterTarget(target); err != nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", err.Error()
			out.Status = "partial"
			out.Items[i] = item
			continue
		}
		normalizedKey := strings.ToLower(key)
		if _, duplicate := seen[normalizedKey]; duplicate {
			return mcpcontract.FindClustersOutput{}, mcpcontract.InvalidArgument("targets", "must not contain duplicate targets", map[string]any{
				"targets": []map[string]string{{"owner": "acme", "repo": "rocket"}},
			})
		}
		seen[normalizedKey] = struct{}{}
		value, err := findClustersTarget(ctx, c, target, in.Limit)
		switch {
		case err == nil:
			if value.Truncated {
				item.Status, item.Reason, item.Message = "partial", "cluster_truncated", "the stored cluster population exceeded the requested bound"
				out.Status = "partial"
				nextLimit := min(100, max(in.Limit*2, in.Limit+1))
				value.Recovery = recoveryPlan("cluster_truncated", "The stored cluster population exceeded this bound. Request a larger cluster limit before treating the returned clusters as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.FindClustersInput{Targets: []mcpcontract.ClusterTarget{target}, Limit: nextLimit, SnapshotToken: in.SnapshotToken}))
			}
			item.Value = &value
		case errors.Is(err, errRepositoryNotFound):
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", err.Error()
			item.Recovery = recoveryPlan("repository_not_indexed", err.Error(), syncRepositoryContextCall(target.Owner, target.Repo))
			out.Status = "partial"
		case errors.Is(err, errThreadNotFound):
			item.Status, item.Reason, item.Message = "unavailable", "thread_not_indexed", err.Error()
			item.Recovery = recoveryPlan("thread_not_indexed", err.Error(), syncThreadCall(mcpcontract.ThreadRef(target)))
			out.Status = "partial"
		default:
			item.Status, item.Reason, item.Message = "failed", "read_failed", err.Error()
			out.Status = "partial"
		}
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.FindClustersOutput{}, err
	}
	return out, nil
}

func findClustersTarget(ctx context.Context, c *corpus.Corpus, target mcpcontract.ClusterTarget, limit int) (mcpcontract.ClusterSetOutput, error) {
	ref := domain.RepoRef{Owner: target.Owner, Repo: target.Repo}
	repository, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.ClusterSetOutput{}, err
	}
	if repository == nil {
		return mcpcontract.ClusterSetOutput{}, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}
	if target.Kind != "" {
		thread, err := c.GetThread(ctx, repository.ID, target.Kind, target.Number)
		if err != nil {
			return mcpcontract.ClusterSetOutput{}, err
		}
		if thread == nil {
			return mcpcontract.ClusterSetOutput{}, fmt.Errorf("%w: %s#%d", errThreadNotFound, ref, target.Number)
		}
		projection, err := c.GetClusterProjectionForMemberWithIdentity(ctx, clustering.MemberRef{Kind: target.Kind, Owner: target.Owner, Repo: target.Repo, Number: target.Number})
		if err != nil {
			return mcpcontract.ClusterSetOutput{}, fmt.Errorf("find cluster member: %w", err)
		}
		out := mcpcontract.ClusterSetOutput{Owner: target.Owner, Repo: target.Repo}
		if len(projection.Clusters) > 0 {
			out.Total = 1
			out.Clusters = []mcpcontract.ClusterOutput{clusterToMCP(projection.Clusters[0], 20)}
		}
		if projection.Projection != nil {
			out.RuleVersion = projection.Projection.RuleVersion
		}
		return out, nil
	}
	projection, err := c.ListClusterProjection(ctx, ref, clustering.ClusterOpen, limit)
	if err != nil {
		return mcpcontract.ClusterSetOutput{}, fmt.Errorf("list clusters: %w", err)
	}
	out := mcpcontract.ClusterSetOutput{
		Owner:     target.Owner,
		Repo:      target.Repo,
		Total:     projection.Total,
		Truncated: projection.Truncated,
		Clusters:  make([]mcpcontract.ClusterOutput, len(projection.Clusters)),
	}
	if projection.Projection != nil {
		out.RuleVersion = projection.Projection.RuleVersion
	}
	for i, cl := range projection.Clusters {
		out.Clusters[i] = clusterToMCP(cl, 20)
	}
	return out, nil
}

func validateClusterTarget(target mcpcontract.ClusterTarget) error {
	if err := (domain.RepoRef{Owner: target.Owner, Repo: target.Repo}).Validate(); err != nil {
		return err
	}
	if (target.Kind == "") != (target.Number == 0) {
		return errors.New("kind and number must be provided together")
	}
	if target.Kind != "" && target.Kind != "issue" && target.Kind != "pull_request" {
		return errors.New("kind must be issue or pull_request")
	}
	if target.Number < 0 {
		return errors.New("number must be positive")
	}
	return nil
}

func clusterTargetKey(target mcpcontract.ClusterTarget) string {
	key := target.Owner + "/" + target.Repo
	if target.Kind != "" && target.Number > 0 {
		return fmt.Sprintf("%s:%s#%d", key, target.Kind, target.Number)
	}
	return key
}

// FindNeighbors ranks similar local threads in input order without network
// access.
func (r *MCPReader) FindNeighbors(ctx context.Context, in mcpcontract.FindNeighborsInput) (mcpcontract.FindNeighborsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 20 {
		return mcpcontract.FindNeighborsOutput{}, mcpcontract.InvalidArgument("threads", "must contain 1 to 20 items", map[string]any{
			"threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 1}},
		})
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.FindNeighborsOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 10})
	}
	out := mcpcontract.FindNeighborsOutput{
		Status: "complete",
		Items:  make([]mcpcontract.BatchItem[mcpcontract.NeighborSetOutput], len(in.Threads)),
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.FindNeighborsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.FindNeighborsOutput{}, err
	}
	out.SnapshotToken = snapshotIdentity(in.SnapshotToken, revision)
	seen := make(map[string]struct{}, len(in.Threads))
	for i, thread := range in.Threads {
		key := fmt.Sprintf("%s/%s:%s#%d", thread.Owner, thread.Repo, thread.Kind, thread.Number)
		item := mcpcontract.BatchItem[mcpcontract.NeighborSetOutput]{Key: key, Status: "complete"}
		if err := validateSimilarityThread(thread); err != nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", err.Error()
			out.Status = "partial"
			out.Items[i] = item
			continue
		}
		normalizedKey := strings.ToLower(key)
		if _, duplicate := seen[normalizedKey]; duplicate {
			return mcpcontract.FindNeighborsOutput{}, mcpcontract.InvalidArgument("threads", "must not contain duplicate threads", map[string]any{
				"threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 1}},
			})
		}
		seen[normalizedKey] = struct{}{}
		result, err := r.Neighbors(ctx, contracts.RepoRef{Owner: thread.Owner, Repo: thread.Repo}, thread.Kind, thread.Number, in.Limit)
		switch {
		case err == nil:
			value := mcpcontract.NeighborSetOutput{
				Owner: thread.Owner, Repo: thread.Repo, Kind: result.Kind, Number: result.Number, SourceRevision: result.SourceRevision,
				Neighbors: make([]mcpcontract.NeighborOutput, len(result.Neighbors)),
			}
			for j, neighbor := range result.Neighbors {
				value.Neighbors[j] = mcpcontract.NeighborOutput{
					Kind: neighbor.Kind, Owner: neighbor.Owner, Repo: neighbor.Repo, Number: neighbor.Number,
					Title: neighbor.Title, State: neighbor.State, Score: mcpcontract.SimilarityScore(neighbor.Score), Reason: neighbor.Reason,
				}
			}
			item.Value = &value
		case errors.Is(err, errRepositoryNotFound):
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", err.Error()
			item.Recovery = recoveryPlan("repository_not_indexed", err.Error(), syncRepositoryContextCall(thread.Owner, thread.Repo))
			out.Status = "partial"
		case errors.Is(err, errThreadNotFound):
			item.Status, item.Reason, item.Message = "unavailable", "thread_not_indexed", err.Error()
			item.Recovery = recoveryPlan("thread_not_indexed", err.Error(), syncThreadCall(mcpcontract.ThreadRef{Owner: thread.Owner, Repo: thread.Repo, Kind: thread.Kind, Number: thread.Number}))
			out.Status = "partial"
		default:
			item.Status, item.Reason, item.Message = "failed", "read_failed", err.Error()
			out.Status = "partial"
		}
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.FindNeighborsOutput{}, err
	}
	return out, nil
}

func validateSimilarityThread(thread mcpcontract.ThreadRef) error {
	if err := (domain.RepoRef{Owner: thread.Owner, Repo: thread.Repo}).Validate(); err != nil {
		return err
	}
	if thread.Kind != "issue" && thread.Kind != "pull_request" {
		return errors.New("kind must be issue or pull_request")
	}
	if thread.Number <= 0 {
		return errors.New("number must be positive")
	}
	return nil
}
