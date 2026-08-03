package corpus

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	testCorpusTemplateOnce sync.Once
	testCorpusTemplatePath string
	errTestCorpusTemplate  error
)

func openTestCorpus(t *testing.T) (*Corpus, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	if err := copyTestDatabase(testCorpusTemplate(t), path); err != nil {
		t.Fatalf("copy corpus template: %v", err)
	}
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, path
}

func testCorpusTemplate(t *testing.T) string {
	t.Helper()
	testCorpusTemplateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gitcontribute-corpus-template-")
		if err != nil {
			errTestCorpusTemplate = err
			return
		}
		testCorpusTemplatePath = filepath.Join(dir, "corpus.db")
		c, err := Open(context.Background(), testCorpusTemplatePath)
		if err != nil {
			errTestCorpusTemplate = err
			return
		}
		if _, err := c.db.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			errTestCorpusTemplate = errors.Join(err, c.Close())
			return
		}
		errTestCorpusTemplate = c.Close()
		if errTestCorpusTemplate != nil {
			return
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if err := os.Remove(testCorpusTemplatePath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				errTestCorpusTemplate = err
				return
			}
		}
	})
	if errTestCorpusTemplate != nil {
		t.Fatalf("initialize corpus template: %v", errTestCorpusTemplate)
	}
	return testCorpusTemplatePath
}

func copyTestDatabase(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return errors.Join(err, out.Close())
	}
	return out.Close()
}

func TestTestCorpusTemplateIsCurrentAndStandalone(t *testing.T) {
	t.Parallel()
	c, path := openTestCorpus(t)
	if _, err := os.Stat(testCorpusTemplate(t) + "-wal"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("template retained a WAL sidecar: %v", err)
	}
	_, target, err := c.SchemaVersions(context.Background())
	if err != nil {
		t.Fatalf("schema versions: %v", err)
	}
	current, exists, err := InspectSchemaVersion(context.Background(), path)
	if err != nil {
		t.Fatalf("inspect copied schema: %v", err)
	}
	if !exists || current != target {
		t.Fatalf("copied schema version = %d (exists=%t), want current %d", current, exists, target)
	}
}
