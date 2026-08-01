package corpus

import (
	"context"
	"errors"
	"testing"
)

func TestReadSnapshotIsImmutableAndUnavailableNeverFallsBack(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	first, err := c.MaterializeReadSnapshot(ctx, SnapshotMaterialization{Kind: "coverage", Scope: map[string]string{"repository": "acme/rocket"}, SourceManifest: map[string]int{"observation": 1}, DerivedVersions: map[string]string{"coverage": "v1"}, Completeness: map[string]bool{"complete": true}, Provenance: map[string]string{"producer": "test"}, Payload: map[string]any{"facets": []string{"metadata"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE corpus_state SET revision=revision+1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	second, err := c.MaterializeReadSnapshot(ctx, SnapshotMaterialization{Kind: "coverage", Scope: map[string]string{"repository": "acme/rocket"}, SourceManifest: map[string]int{"observation": 2}, DerivedVersions: map[string]string{"coverage": "v1"}, Completeness: map[string]bool{"complete": false}, Provenance: map[string]string{"producer": "test"}, Payload: map[string]any{"facets": []string{"metadata", "threads"}}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token || first.ArtifactDigest == second.ArtifactDigest {
		t.Fatalf("snapshot identities collapsed: first=%+v second=%+v", first, second)
	}
	reread, err := c.ResolveReadSnapshot(ctx, first.Token)
	if err != nil || string(reread.Payload) != string(first.Payload) {
		t.Fatalf("old snapshot changed: %+v, %v", reread, err)
	}
	if _, err := c.ResolveReadSnapshot(ctx, "missing"); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("missing snapshot error = %v", err)
	}
}

func TestReadSnapshotRejectsInconsistentArtifact(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	snapshot, err := c.MaterializeReadSnapshot(ctx, SnapshotMaterialization{Kind: "coverage", Scope: "scope", SourceManifest: "source", DerivedVersions: map[string]string{}, Completeness: map[string]bool{}, Provenance: map[string]string{}, Payload: map[string]string{"value": "original"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE corpus_read_artifacts SET payload_json='{"value":"tampered"}' WHERE digest=?`, snapshot.ArtifactDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveReadSnapshot(ctx, snapshot.Token); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("tampered snapshot error = %v", err)
	}
}

func TestResolveReadArtifactUsesExactKindAndDigestWithoutProjectionFallback(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	want, err := c.MaterializeReadSnapshot(ctx, SnapshotMaterialization{
		Kind: "source-bundle.v1", Scope: "acme/rocket", SourceManifest: "manifest",
		DerivedVersions: map[string]string{"source_bundle": "v1"}, Completeness: map[string]bool{"complete": true},
		Provenance: map[string]string{"provider": "github"}, Payload: map[string]any{"commit_sha": "abc", "items": []string{"README.md"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ResolveReadArtifact(ctx, "source-bundle.v1", want.ArtifactDigest)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArtifactKind != want.ArtifactKind || got.ArtifactDigest != want.ArtifactDigest || string(got.Payload) != string(want.Payload) || got.Token != want.Token {
		t.Fatalf("artifact = %+v, want %+v", got, want)
	}
	if _, err := c.ResolveReadArtifact(ctx, "source-bundle.v1", "missing"); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("malformed artifact error = %v", err)
	}
	if _, err := c.ResolveReadArtifact(ctx, "other-kind", want.ArtifactDigest); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("wrong-kind artifact error = %v", err)
	}
}
