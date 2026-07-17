package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
)

// Clusters computes and lists duplicate-candidate clusters for a repository.
func (s *Service) Clusters(ctx context.Context, repo cli.RepoRef, limit int) (*cli.ClusterListResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		return nil, errors.New("cluster limit cannot exceed 1000")
	}

	_, clusters, err := c.Clustering().ComputeForRepo(ctx, ref, clustering.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("compute clusters: %w", err)
	}

	result := &cli.ClusterListResult{
		Repo:  repo,
		Total: len(clusters),
	}
	for i, cl := range clusters {
		if i >= limit {
			break
		}
		result.Clusters = append(result.Clusters, *clusterToCLI(cl, 0))
	}
	return result, nil
}

// Cluster returns a single cluster by stable id.
func (s *Service) Cluster(ctx context.Context, id string, limit int) (*cli.ClusterResult, error) {
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

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	cl, err := c.Clustering().GetCluster(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if cl == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("cluster %q not found", id))
	}
	return clusterToCLI(*cl, limit), nil
}

func clusterToCLI(cl clustering.Cluster, memberLimit int) *cli.ClusterResult {
	var members []cli.ClusterMember
	if memberLimit > 0 {
		members = make([]cli.ClusterMember, 0, len(cl.Members))
		for count, m := range cl.Members {
			if count >= memberLimit {
				break
			}
			members = append(members, cli.ClusterMember{
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
	return &cli.ClusterResult{
		StableID:    cl.StableID,
		State:       string(cl.State),
		Canonical:   cli.ClusterMember{Kind: cl.Canonical.Kind, Owner: cl.Canonical.Owner, Repo: cl.Canonical.Repo, Number: cl.Canonical.Number},
		MemberCount: len(cl.Members),
		Members:     members,
	}
}
