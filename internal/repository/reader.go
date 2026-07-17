// Package repository defines narrow, product-owned repository read contracts
// shared by offline research builders.
package repository

import (
	"context"

	"github.com/morluto/gitcontribute/internal/domain"
)

// Reader exposes source-backed repository facts that can be shared by
// dossiers and thread research briefs. Implementations must not perform
// network access, local writes, or process execution.
type Reader interface {
	ReadRepository(ctx context.Context, ref domain.RepoRef) (domain.Repository, []domain.SourceRef, error)
	ReadCoverage(ctx context.Context, ref domain.RepoRef) (domain.Coverage, error)
	ReadContributionGuidance(ctx context.Context, ref domain.RepoRef) (string, []domain.SourceRef, error)
}
