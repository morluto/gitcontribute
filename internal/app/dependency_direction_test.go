package app

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestApplicationDoesNotImportInboundAdapters(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"github.com/morluto/gitcontribute/internal/cli",
		"github.com/morluto/gitcontribute/internal/mcpserver",
		"github.com/morluto/gitcontribute/internal/tui",
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			for _, adapter := range forbidden {
				if importPath == adapter {
					t.Errorf("%s imports inbound adapter %s", path, adapter)
				}
			}
		}
	}
}
