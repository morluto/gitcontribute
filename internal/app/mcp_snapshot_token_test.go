package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestSnapshotTokenReadFailsClosedAfterCorpusMutation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, err := svc.corpus.ApplyRepositoryObservation(ctx, "acme", "rocket", "repo-1", time.Unix(1, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.corpus.MaterializeReadSnapshot(ctx, corpus.SnapshotMaterialization{
		Kind: "thread_search", Scope: "acme/rocket", SourceManifest: map[string]int64{"observation_watermark": 1},
		DerivedVersions: map[string]string{"search": "v1"}, Completeness: map[string]bool{"complete": true},
		Provenance: map[string]string{"producer": "test"}, Payload: map[string]any{"query": "rocket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.ApplyRepositoryObservation(ctx, "acme", "rocket", "repo-1", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}

	_, err = svc.MCPReader().Search(ctx, mcpcontract.SearchInput{Query: "rocket", SnapshotToken: snapshot.Token})
	var toolErr *mcpcontract.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != "snapshot_expired" {
		t.Fatalf("stale snapshot error = %v", err)
	}

	_, err = svc.MCPReader().Search(ctx, mcpcontract.SearchInput{Query: "rocket", SnapshotToken: "missing-token"})
	toolErr = nil
	if !errors.As(err, &toolErr) || toolErr.Code != "snapshot_unavailable" {
		t.Fatalf("missing snapshot error = %v", err)
	}

	fresh, err := svc.MCPReader().Search(ctx, mcpcontract.SearchInput{Query: "rocket"})
	if err != nil || fresh.SnapshotToken == "" {
		t.Fatalf("unpinned read = %+v, err=%v", fresh, err)
	}
	_, err = svc.MCPReader().Search(ctx, mcpcontract.SearchInput{Query: "rocket", SnapshotToken: fresh.SnapshotToken})
	toolErr = nil
	if !errors.As(err, &toolErr) || toolErr.Code != "snapshot_unavailable" {
		t.Fatalf("ephemeral snapshot reuse error = %v", err)
	}
}
