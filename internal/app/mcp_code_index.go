package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CodeIndexArtifact returns the canonical read-plane handoff for one exact
// indexed commit. It reads only the local corpus and binds the manifest and
// identity to the same logical corpus revision.
func (r *MCPReader) CodeIndexArtifact(ctx context.Context, owner, repo, commit string) (mcpcontract.CodeIndexArtifact, error) {
	ref := domain.RepoRef{Owner: owner, Repo: repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	if commit == "" {
		return mcpcontract.CodeIndexArtifact{}, fmt.Errorf("commit_sha is required")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	revision, err := beginCorpusRead(ctx, c, nil)
	if err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	snapshot, err := c.CodeSnapshot(ctx, ref, commit)
	if err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	if snapshot == nil {
		return mcpcontract.CodeIndexArtifact{}, mcpcontract.ErrNotFound
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.CodeIndexArtifact{}, err
	}
	return codeIndexArtifact(ref, snapshot.CommitSHA, snapshot.Manifest, revision), nil
}

func codeIndexArtifact(ref domain.RepoRef, commit string, manifest codeindex.Manifest, revision int64) mcpcontract.CodeIndexArtifact {
	manifestBytes, _ := json.Marshal(manifest)
	manifestDigest := sha256.Sum256(manifestBytes)
	manifestSHA := hex.EncodeToString(manifestDigest[:])
	identityBytes := []byte(ref.String() + "\x00" + commit + "\x00" + manifestSHA)
	identityDigest := sha256.Sum256(identityBytes)
	identity := "code-index:" + hex.EncodeToString(identityDigest[:])
	manifestID := "code-index-manifest:" + ref.String() + "@" + commit
	uri := fmt.Sprintf("gitcontribute://code-index/%s/%s/%s", ref.Owner, ref.Repo, commit)
	followUp := &mcpcontract.JobFollowUp{
		ResourceURI: uri,
		Arguments:   &mcpcontract.ToolCallArguments{Repository: ref.String(), Commit: commit},
		Reason:      "Read the exact indexed-commit artifact through MCP resources/read.",
	}
	return mcpcontract.CodeIndexArtifact{
		Kind: "code_index", ID: identity, Repository: mcpcontract.RepositoryRef{Owner: ref.Owner, Repo: ref.Repo},
		CommitSHA: commit, CorpusRevision: revision, ManifestID: manifestID, ManifestSHA256: manifestSHA,
		FileCount: mcpcontract.NonNegativeInt(manifest.IndexedFiles), TrackedEntries: mcpcontract.NonNegativeInt(manifest.TrackedEntries),
		Truncated: manifest.Truncated, ResourceURI: uri, FollowUp: followUp,
	}
}
