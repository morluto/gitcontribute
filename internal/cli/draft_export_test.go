package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func TestExportDraftPreservesExactBytes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prepared")
	draft := &contracts.DraftResult{
		ID: "draft-1", Revision: 2, Title: "Unicode ✓", Body: "line one\r\nline two\n",
		TitleSHA256: "title-digest", BodySHA256: "body-digest",
	}
	if err := exportDraft(dir, draft, false); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"title.txt": draft.Title, "body.md": draft.Body} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want exact %q", name, got, want)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded contracts.DraftResult
	if err := json.Unmarshal(metadata, &decoded); err != nil || decoded.ID != draft.ID || decoded.Revision != draft.Revision {
		t.Fatalf("metadata = %+v, %v", decoded, err)
	}
}

func TestExportDraftRequiresWarningOverrideAndRejectsErrors(t *testing.T) {
	for _, tc := range []struct {
		severity string
		allow    bool
		wantErr  bool
	}{
		{severity: "warning", allow: false, wantErr: true},
		{severity: "warning", allow: true, wantErr: false},
		{severity: "error", allow: true, wantErr: true},
	} {
		draft := &contracts.DraftResult{
			ID: "draft", Revision: 1, Title: "title", Body: "body",
			Warnings: []contracts.DraftDiagnosticResult{{Code: "finding", Severity: tc.severity, Message: "message"}},
		}
		err := exportDraft(filepath.Join(t.TempDir(), tc.severity), draft, tc.allow)
		if (err != nil) != tc.wantErr {
			t.Fatalf("severity=%s allow=%v error=%v", tc.severity, tc.allow, err)
		}
	}
}
