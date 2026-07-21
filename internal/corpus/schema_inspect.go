package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// InspectSchemaVersion reads the applied schema version from an existing
// corpus without creating the database or applying migrations. The boolean is
// false when path has no persistent database to inspect.
func InspectSchemaVersion(ctx context.Context, path string) (int64, bool, error) {
	filePath, dsn, inspectable, err := schemaInspectionTarget(path)
	if err != nil || !inspectable {
		return 0, false, err
	}
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("inspect corpus file: %w", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return 0, false, fmt.Errorf("open corpus read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	version, _, schemaErr := (&Corpus{db: db}).inspectSchemaVersions(ctx)
	closeErr := db.Close()
	if schemaErr != nil {
		return 0, false, fmt.Errorf("inspect corpus schema version: %w", schemaErr)
	}
	if closeErr != nil {
		return 0, false, fmt.Errorf("close read-only corpus: %w", closeErr)
	}
	return version, true, nil
}

func (c *Corpus) inspectSchemaVersions(ctx context.Context) (current, target int64, err error) {
	target, err = latestSchemaVersion()
	if err != nil {
		return 0, 0, err
	}
	var tableCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'`).Scan(&tableCount); err != nil {
		return 0, 0, fmt.Errorf("inspect migration table: %w", err)
	}
	if tableCount == 0 {
		return 0, target, nil
	}
	if err := c.db.QueryRowContext(ctx, `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, target, nil
		}
		return 0, 0, fmt.Errorf("inspect applied schema version: %w", err)
	}
	return current, target, nil
}

func latestSchemaVersion() (int64, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return 0, fmt.Errorf("list embedded migrations: %w", err)
	}
	var latest int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, parseErr := strconv.ParseInt(prefix, 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("parse migration version from %q: %w", entry.Name(), parseErr)
		}
		if version > latest {
			latest = version
		}
	}
	return latest, nil
}

func schemaInspectionTarget(path string) (filePath, dsn string, inspectable bool, err error) {
	if path == "" || path == ":memory:" {
		return "", "", false, nil
	}
	if !strings.HasPrefix(path, "file:") {
		slashPath := filepath.ToSlash(path)
		if runtime.GOOS == "windows" && filepath.IsAbs(path) && !strings.HasPrefix(slashPath, "/") {
			slashPath = "/" + slashPath
		}
		u := &url.URL{Scheme: "file", Path: slashPath}
		query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "foreign_keys(1)", "busy_timeout(5000)"}}
		u.RawQuery = query.Encode()
		return path, u.String(), true, nil
	}

	u, err := url.Parse(path)
	if err != nil {
		return "", "", false, fmt.Errorf("parse corpus URI: %w", err)
	}
	query := u.Query()
	if query.Get("mode") == "memory" || u.Path == ":memory:" || u.Opaque == ":memory:" {
		return "", "", false, nil
	}
	filePath = u.Path
	if filePath == "" {
		filePath = u.Opaque
	}
	if u.Host != "" && u.Host != "localhost" {
		filePath = "//" + u.Host + "/" + strings.TrimPrefix(filePath, "/")
	}
	if runtime.GOOS == "windows" && len(filePath) >= 3 && filePath[0] == '/' && filePath[2] == ':' {
		filePath = filePath[1:]
	}
	filePath = filepath.FromSlash(filePath)
	if filePath == "" {
		return "", "", false, errors.New("corpus URI has no file path")
	}

	query.Set("mode", "ro")
	query.Del("_pragma")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()
	return filePath, u.String(), true, nil
}

// CheckWriteAccessAtPath checks whether an existing compatible corpus can
// begin a write transaction without opening the migration-capable corpus path.
// The transaction is always rolled back and no schema or archive data changes.
func CheckWriteAccessAtPath(ctx context.Context, path string) error {
	filePath, readOnlyDSN, inspectable, err := schemaInspectionTarget(path)
	if err != nil {
		return err
	}
	if !inspectable {
		return errors.New("write-access inspection requires a persistent database path")
	}
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("inspect corpus for write access: %w", err)
	}
	lease, err := acquireCorpusLease(path, false, "check corpus write access")
	if err != nil {
		return err
	}
	defer lease.release()
	u, err := url.Parse(readOnlyDSN)
	if err != nil {
		return fmt.Errorf("parse corpus inspection URI: %w", err)
	}
	query := u.Query()
	query.Set("mode", "rw")
	query.Del("_pragma")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(0)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return fmt.Errorf("open corpus for write-access inspection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	c := &Corpus{db: db}
	checkErr := c.CheckWriteAccess(ctx)
	closeErr := db.Close()
	if checkErr != nil {
		return checkErr
	}
	if closeErr != nil {
		return fmt.Errorf("close write-access inspection: %w", closeErr)
	}
	return nil
}
