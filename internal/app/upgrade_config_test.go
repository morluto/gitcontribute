package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadClaudeCommandRejectsNonStringArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	data := `{"mcpServers":{"gitcontribute":{"command":"node","args":["mcp",123]}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClaudeCommand(path); err == nil || !strings.Contains(err.Error(), "args[1]") {
		t.Fatalf("readClaudeCommand error = %v, want indexed non-string argument error", err)
	}
}
