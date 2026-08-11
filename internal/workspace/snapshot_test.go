package workspace

import (
	"encoding/json"
	"testing"
)

func TestSnapshotUnmarshalRejectsContradictoryDerivedFlags(t *testing.T) {
	for name, payload := range map[string]string{
		"commit truncation": `{"commit_total":1,"commits":[],"commits_truncated":false,"complete":true,"gaps":[]}`,
		"completeness":      `{"commit_total":0,"commits":[],"commits_truncated":false,"complete":true,"gaps":[{"code":"unbound","reason":"missing"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var snapshot Snapshot
			if err := json.Unmarshal([]byte(payload), &snapshot); err == nil {
				t.Fatal("contradictory snapshot flags were accepted")
			}
		})
	}
}

func TestSnapshotUnmarshalRejectsUnknownOwnedKinds(t *testing.T) {
	for name, payload := range map[string]string{
		"ownership": `{"ownership":"borrowed","commit_total":0,"commits":[],"commits_truncated":false,"complete":true,"gaps":[]}`,
		"resource":  `{"ownership":"managed","untracked":[{"path":"note.txt","kind":"directory"}],"commit_total":0,"commits":[],"commits_truncated":false,"complete":true,"gaps":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var snapshot Snapshot
			if err := json.Unmarshal([]byte(payload), &snapshot); err == nil {
				t.Fatal("unknown workspace discriminator was accepted")
			}
		})
	}
}

func TestWorkspaceUnmarshalRejectsCleanStateWithUntrackedFiles(t *testing.T) {
	var workspace Workspace
	if err := json.Unmarshal([]byte(`{"Dirty":false,"HasUntracked":true}`), &workspace); err == nil {
		t.Fatal("contradictory workspace change flags were accepted")
	}
}

func TestWorkspaceUnmarshalParsesOwnership(t *testing.T) {
	for name, payload := range map[string]string{
		"legacy managed": `{"Dirty":false,"HasUntracked":false}`,
		"external":       `{"Dirty":false,"HasUntracked":false,"Ownership":"external"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var workspace Workspace
			if err := json.Unmarshal([]byte(payload), &workspace); err != nil {
				t.Fatal(err)
			}
			want := OwnershipManaged
			if name == "external" {
				want = OwnershipExternal
			}
			if workspace.Ownership != want {
				t.Fatalf("ownership = %q, want %q", workspace.Ownership, want)
			}
		})
	}

	var workspace Workspace
	if err := json.Unmarshal([]byte(`{"Dirty":false,"HasUntracked":false,"Ownership":"borrowed"}`), &workspace); err == nil {
		t.Fatal("unknown workspace ownership was accepted")
	}
}
