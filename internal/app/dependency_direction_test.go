package app

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductPackagesDoNotImportInboundAdapters(t *testing.T) {
	t.Parallel()
	productPackages := []string{
		".",
		"../contracts",
		"../failure",
		"../mcpcontract",
		"../tuicontract",
	}
	forbidden := []string{
		"github.com/morluto/gitcontribute/internal/cli",
		"github.com/morluto/gitcontribute/internal/mcpserver",
		"github.com/morluto/gitcontribute/internal/tui",
	}
	for _, dir := range productPackages {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
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
}
