package corpus

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectSchemaVersionDoesNotCreateMissingCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	version, exists, err := InspectSchemaVersion(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if exists || version != 0 {
		t.Fatalf("missing corpus inspection = version:%d exists:%v", version, exists)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("inspection created missing corpus: %v", err)
	}
}

func TestInspectSchemaVersionReadsExistingCorpusWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus #1.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got, exists, err := InspectSchemaVersion(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got != want {
		t.Fatalf("schema inspection = version:%d exists:%v, want version:%d", got, exists, want)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only schema inspection mutated the corpus")
	}
}

func TestInspectSchemaVersionSupportsFileURI(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	got, exists, err := InspectSchemaVersion(ctx, "file:"+filepath.ToSlash(path)+"?cache=private")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got != want {
		t.Fatalf("URI schema inspection = version:%d exists:%v, want version:%d", got, exists, want)
	}
}
