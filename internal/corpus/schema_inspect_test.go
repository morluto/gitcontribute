package corpus

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenReadOnlyDoesNotCreateMissingCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(context.Background(), path); err == nil {
		t.Fatal("OpenReadOnly unexpectedly opened a missing corpus")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created missing corpus: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created lease file: %v", err)
	}
}

func TestOpenReadOnlyReadsCurrentCorpus(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "current.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if _, err := readOnly.db.ExecContext(ctx, `INSERT INTO repositories (owner, name, created_at, updated_at) VALUES ('owner', 'repo', 1, 1)`); err == nil {
		t.Fatal("read-only corpus accepted a write")
	}
}

func TestOpenReadOnlyRejectsNewerSchemaWithoutDatabaseOrWALMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "newer.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	current, err := c.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)`, current+1); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	type snapshot struct {
		data    []byte
		modTime time.Time
	}
	before := make(map[string]snapshot)
	for _, candidate := range []string{path, path + "-wal"} {
		data, readErr := os.ReadFile(candidate)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		info, statErr := os.Stat(candidate)
		if statErr != nil {
			t.Fatal(statErr)
		}
		before[candidate] = snapshot{data: data, modTime: info.ModTime()}
	}

	_, err = OpenReadOnly(ctx, path)
	var unsupported *UnsupportedSchemaError
	if !errors.As(err, &unsupported) {
		t.Fatalf("OpenReadOnly error = %v, want UnsupportedSchemaError", err)
	}
	for candidate, want := range before {
		got, readErr := os.ReadFile(candidate)
		if readErr != nil {
			t.Fatal(readErr)
		}
		info, statErr := os.Stat(candidate)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !bytes.Equal(got, want.data) || !info.ModTime().Equal(want.modTime) {
			t.Fatalf("read-only open mutated %s", candidate)
		}
	}
}

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
