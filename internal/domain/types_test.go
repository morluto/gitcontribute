package domain

import (
	"errors"
	"testing"
)

func TestRepoRefValidate(t *testing.T) {
	cases := []struct {
		name    string
		ref     RepoRef
		wantErr error
	}{
		{
			name:    "valid owner and repo",
			ref:     RepoRef{Owner: "golang", Repo: "go"},
			wantErr: nil,
		},
		{
			name:    "owner with hyphen",
			ref:     RepoRef{Owner: "some-owner", Repo: "repo-name"},
			wantErr: nil,
		},
		{
			name:    "repo with dot",
			ref:     RepoRef{Owner: "owner", Repo: "repo.go"},
			wantErr: nil,
		},
		{
			name:    "empty owner",
			ref:     RepoRef{Owner: "", Repo: "go"},
			wantErr: errOwnerEmpty,
		},
		{
			name:    "empty repo",
			ref:     RepoRef{Owner: "golang", Repo: ""},
			wantErr: errRepoEmpty,
		},
		{
			name:    "owner starts with hyphen",
			ref:     RepoRef{Owner: "-bad", Repo: "go"},
			wantErr: errors.New("invalid owner \"-bad\""),
		},
		{
			name:    "repo is path traversal",
			ref:     RepoRef{Owner: "golang", Repo: "../go"},
			wantErr: errors.New("invalid repo \"../go\""),
		},
		{
			name:    "repo is dot",
			ref:     RepoRef{Owner: "golang", Repo: "."},
			wantErr: errors.New("invalid repo \".\""),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ref.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Fatalf("expected error %q, got %v", tc.wantErr, err)
			}
		})
	}
}
