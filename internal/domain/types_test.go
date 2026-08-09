package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNewRepoRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		owner   string
		repo    string
		wantErr error
	}{
		{
			name:    "valid owner and repo",
			owner:   "golang",
			repo:    "go",
			wantErr: nil,
		},
		{
			name:    "owner with hyphen",
			owner:   "some-owner",
			repo:    "repo-name",
			wantErr: nil,
		},
		{
			name:    "repo with dot",
			owner:   "owner",
			repo:    "repo.go",
			wantErr: nil,
		},
		{
			name:    "empty owner",
			repo:    "go",
			wantErr: errOwnerEmpty,
		},
		{
			name:    "empty repo",
			owner:   "golang",
			wantErr: errRepoEmpty,
		},
		{
			name:    "owner starts with hyphen",
			owner:   "-bad",
			repo:    "go",
			wantErr: errors.New("invalid owner \"-bad\""),
		},
		{
			name:    "repo is path traversal",
			owner:   "golang",
			repo:    "../go",
			wantErr: errors.New("invalid repo \"../go\""),
		},
		{
			name:    "repo is dot",
			owner:   "golang",
			repo:    ".",
			wantErr: errors.New("invalid repo \".\""),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := NewRepoRef(tc.owner, tc.repo)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if ref.Owner() != tc.owner || ref.Repo() != tc.repo || ref.String() != tc.owner+"/"+tc.repo {
					t.Fatalf("repository reference = %#v", ref)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Fatalf("expected error %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestParseRepoRefAndJSONRejectInvalidRepresentations(t *testing.T) {
	t.Parallel()
	ref, err := ParseRepoRef(" golang/go ")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RepoRef
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != ref {
		t.Fatalf("decoded = %v, want %v", decoded, ref)
	}
	for _, input := range []string{"", "owner", "owner/repo/extra"} {
		if _, err := ParseRepoRef(input); err == nil {
			t.Errorf("ParseRepoRef(%q) succeeded", input)
		}
	}
	for _, payload := range []string{
		`{"Owner":"owner","Repo":""}`,
		`{"Owner":"owner","Repo":"repo","Unexpected":true}`,
	} {
		if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Errorf("json.Unmarshal(%s) succeeded", payload)
		}
	}
}

func TestRepositoryJSONKeepsIdentityAndSnapshotFields(t *testing.T) {
	t.Parallel()
	want := Repository{Ref: MustRepoRef("golang", "go"), Description: "Go", Stars: 42}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if encoded := string(payload); !strings.Contains(encoded, `"Owner":"golang"`) || !strings.Contains(encoded, `"Repo":"go"`) || strings.Contains(encoded, `"Ref"`) {
		t.Fatalf("repository JSON changed its durable identity shape: %s", encoded)
	}
	var got Repository
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Ref != want.Ref || got.Description != want.Description || got.Stars != want.Stars {
		t.Fatalf("repository round trip = %+v, want %+v", got, want)
	}

	var legacy Repository
	if err := json.Unmarshal([]byte(`{"Owner":"golang","Repo":"go","Description":"stored dossier","Stars":7}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Ref != MustRepoRef("golang", "go") || legacy.Description != "stored dossier" || legacy.Stars != 7 {
		t.Fatalf("legacy repository JSON = %+v", legacy)
	}
}

func TestParseThreadKindAndStateRejectUnknownVariants(t *testing.T) {
	t.Parallel()
	if kind, err := ParseThreadKind(" pull_request "); err != nil || kind != PullRequestKind {
		t.Fatalf("parse thread kind = %q, %v", kind, err)
	}
	if state, err := ParseThreadState(" open "); err != nil || state != OpenState {
		t.Fatalf("parse thread state = %q, %v", state, err)
	}
	if _, err := ParseThreadKind("discussion"); err == nil {
		t.Fatal("unknown thread kind was accepted")
	}
	if _, err := ParseThreadState("draft"); err == nil {
		t.Fatal("unknown thread state was accepted")
	}
}
