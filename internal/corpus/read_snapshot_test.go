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
	first, err := c.MaterializeReadSnapshot(ctx, mustSnapshotMaterialization(t, "coverage", map[string]string{"repository": "acme/rocket"}, map[string]int{"observation": 1}, map[string]string{"coverage": "v1"}, map[string]bool{"complete": true}, map[string]string{"producer": "test"}, map[string]any{"facets": []string{"metadata"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE corpus_state SET revision=revision+1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	second, err := c.MaterializeReadSnapshot(ctx, mustSnapshotMaterialization(t, "coverage", map[string]string{"repository": "acme/rocket"}, map[string]int{"observation": 2}, map[string]string{"coverage": "v1"}, map[string]bool{"complete": false}, map[string]string{"producer": "test"}, map[string]any{"facets": []string{"metadata", "threads"}}))
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
	snapshot, err := c.MaterializeReadSnapshot(ctx, mustSnapshotMaterialization(t, "coverage", "scope", "source", map[string]string{}, map[string]bool{}, map[string]string{}, map[string]string{"value": "original"}))
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

func TestReadSnapshotRequiresParsedMaterialization(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	if _, err := c.MaterializeReadSnapshot(context.Background(), SnapshotMaterialization{kind: "coverage"}); err == nil || err.Error() != "snapshot materialization is not parsed" {
		t.Fatalf("unparsed materialization error = %v", err)
	}
	if _, err := NewSnapshotMaterialization("coverage", "scope", "source", struct{}{}, struct{}{}, struct{}{}, make(chan int)); err == nil {
		t.Fatal("unencodable snapshot payload was accepted")
	}
}

func TestResolveReadArtifactUsesExactKindAndDigestWithoutProjectionFallback(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	want, err := c.MaterializeReadSnapshot(ctx, mustSnapshotMaterialization(
		t, "source-bundle.v1", "acme/rocket", "manifest", map[string]string{"source_bundle": "v1"}, map[string]bool{"complete": true},
		map[string]string{"provider": "github"}, map[string]any{"commit_sha": "abc", "items": []string{"README.md"}},
	))
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

func mustSnapshotMaterialization[S, M, D, C, P, V any](t *testing.T, kind string, scope S, source M, derived D, completeness C, provenance P, payload V) SnapshotMaterialization {
	t.Helper()
	materialization, err := NewSnapshotMaterialization(kind, scope, source, derived, completeness, provenance, payload)
	if err != nil {
		t.Fatal(err)
	}
	return materialization
}
