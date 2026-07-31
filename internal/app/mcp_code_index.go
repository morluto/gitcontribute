package app

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CodeIndexArtifact resolves only an immutable digest-bound record. Missing or
// inconsistent records never fall back to the latest repository projection.
func (r *MCPReader) CodeIndexArtifact(ctx context.Context, digest string) (mcpcontract.CodeIndexArtifact, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	record, err := c.CodeIndexArtifact(ctx, digest)
	if err != nil {
		return mcpcontract.CodeIndexArtifact{}, fmt.Errorf("read immutable code-index artifact: %w", err)
	}
	if record == nil {
		return mcpcontract.CodeIndexArtifact{}, mcpcontract.ErrNotFound
	}
	return codeIndexArtifact(*record), nil
}

func codeIndexArtifact(record corpus.CodeIndexArtifactRecord) mcpcontract.CodeIndexArtifact {
	uri := "gitcontribute://artifact/code-index/" + record.Digest
	followUp := &mcpcontract.JobFollowUp{Action: mcpcontract.FollowUpAction{Type: "read_resource", ReadResource: &mcpcontract.ResourceReadAction{URI: uri}}, Reason: "Read this exact digest-bound artifact through MCP resources/read."}
	documents := make([]mcpcontract.CodeIndexDocumentOutput, len(record.Documents))
	for i, document := range record.Documents {
		documents[i] = mcpcontract.CodeIndexDocumentOutput{Path: document.Path, SHA256: document.SHA256, Bytes: mcpcontract.NonNegativeInt(document.Bytes), Language: document.Language}
	}
	return mcpcontract.CodeIndexArtifact{
		Kind: "code_index", ID: "code-index:" + record.Digest,
		Repository: mcpcontract.RepositoryRef{Owner: record.Repo.Owner, Repo: record.Repo.Repo},
		CommitSHA:  record.CommitSHA, SnapshotToken: record.SnapshotToken,
		ManifestID: "code-index-manifest:" + record.ManifestSHA256, ManifestSHA256: record.ManifestSHA256,
		CoverageKnown: record.CoverageKnown,
		Manifest:      mcpcontract.CodeIndexManifestOutput{FormatVersion: record.IndexManifest.FormatVersion, CoverageKnown: record.IndexManifest.CoverageKnown, TrackedEntries: record.IndexManifest.TrackedEntries, IndexedFiles: record.IndexManifest.IndexedFiles, SkippedInvalidPath: record.IndexManifest.SkippedInvalidPath, SkippedExcluded: record.IndexManifest.SkippedExcluded, SkippedNonRegular: record.IndexManifest.SkippedNonRegular, SkippedOversize: record.IndexManifest.SkippedOversize, SkippedTotalBudget: record.IndexManifest.SkippedTotalBudget, SkippedNonText: record.IndexManifest.SkippedNonText, SkippedFileLimit: record.IndexManifest.SkippedFileLimit, Truncated: record.IndexManifest.Truncated},
		SchemaVersion: record.SchemaVersion, TotalBytes: mcpcontract.NonNegativeInt(record.TotalBytes), Documents: documents,
		CreatedAt: record.CreatedAt.Format(time.RFC3339Nano), Provenance: maps.Clone(record.Provenance),
		FileCount: mcpcontract.NonNegativeInt(record.IndexedFiles), TrackedEntries: mcpcontract.NonNegativeInt(record.TrackedEntries),
		Truncated: record.Truncated, ResourceURI: uri, FollowUp: followUp,
	}
}
