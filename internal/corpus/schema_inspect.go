package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

	version, schemaErr := (&Corpus{db: db}).SchemaVersion(ctx)
	closeErr := db.Close()
	if schemaErr != nil {
		return 0, false, fmt.Errorf("inspect corpus schema version: %w", schemaErr)
	}
	if closeErr != nil {
		return 0, false, fmt.Errorf("close read-only corpus: %w", closeErr)
	}
	return version, true, nil
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
		query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
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
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()
	return filePath, u.String(), true, nil
}
