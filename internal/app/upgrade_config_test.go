package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientsetup "github.com/morluto/gitcontribute/internal/setup"
)

func TestReadClaudeCommandRejectsNonStringArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	data := `{"mcpServers":{"gitcontribute":{"command":"node","args":["mcp",123]}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientsetup.ReadCommandFile(clientsetup.Claude, path); err == nil || !strings.Contains(err.Error(), "args[1]") {
		t.Fatalf("readClaudeCommand error = %v, want indexed non-string argument error", err)
	}
}
