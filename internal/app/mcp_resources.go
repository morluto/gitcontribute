package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// Concern reads one persisted concern without external access.
func (r *MCPReader) Concern(ctx context.Context, in mcpcontract.ConcernInput) (mcpcontract.ConcernOutput, error) {
	svc, err := r.readConcernService(ctx)
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	item, err := svc.Get(ctx, in.ID)
	if err != nil {
		return mcpcontract.ConcernOutput{}, mapConcernError(err)
	}
	result, err := r.concernResult(ctx, item)
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// Draft reads one immutable persisted contribution-draft revision.
func (r *MCPReader) Draft(ctx context.Context, in mcpcontract.DraftInput) (mcpcontract.DraftOutput, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.DraftOutput{}, err
	}
	item, err := c.GetContributionDraftRevision(ctx, in.ID, in.Revision)
	if err != nil {
		return mcpcontract.DraftOutput{}, err
	}
	return draftArtifactToMCP(item), nil
}

// Manifest reads one persisted contribution evidence manifest.
func (r *MCPReader) Manifest(ctx context.Context, in mcpcontract.ManifestInput) (mcpcontract.ManifestOutput, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ManifestOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.ManifestOutput{}, err
	}
	statement, err := c.GetContributionManifest(ctx, in.ID)
	if err != nil {
		return mcpcontract.ManifestOutput{}, err
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.ManifestOutput{}, err
	}
	return manifestStatementToMCP(statement, snapshotIdentity(in.SnapshotToken, revision)), nil
}
