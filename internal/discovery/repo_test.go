package discovery

import (
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestParseRepoRef(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    domain.RepoRef
		wantErr string
	}{
		{
			name:  "owner/repo",
			input: "golang/go",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:  "https url",
			input: "https://github.com/golang/go",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:  "http url with query",
			input: "http://github.com/golang/go?tab=readme",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:  "github.com prefix",
			input: "github.com/golang/go",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:  "git ssh",
			input: "git@github.com:golang/go.git",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:  "ssh url",
			input: "ssh://git@github.com/golang/go.git",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "empty repo reference",
		},
		{
			name:    "no slash",
			input:   "golang",
			wantErr: "invalid repo reference",
		},
		{
			name:    "too many segments",
			input:   "github.com/golang/go/issues",
			wantErr: "invalid repo reference",
		},
		{
			name:    "path traversal",
			input:   "golang/../go",
			wantErr: "invalid repo",
		},
		{
			name:    "invalid owner",
			input:   "-bad/go",
			wantErr: "invalid owner",
		},
		{
			name:    "unsupported host",
			input:   "https://gitlab.com/golang/go",
			wantErr: "unsupported host",
		},
		{
			name:    "github subdomain is not a repository host",
			input:   "https://gist.github.com/golang/go",
			wantErr: "unsupported host",
		},
		{
			name:  "uppercase git suffix",
			input: "https://github.com/golang/go.GIT",
			want:  domain.RepoRef{Owner: "golang", Repo: "go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRepoRef(tc.input)
			if tc.wantErr != "" {
				if err == nil || !contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseRepoRef(%q) error = %v, want containing %q", tc.input, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoRef(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseRepoRef(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseRepoRefs(t *testing.T) {
	refs, err := ParseRepoRefs([]string{"a/b", "c/d", "bad"})
	if err == nil {
		t.Fatal("expected error for bad ref")
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 parsed refs, got %d", len(refs))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) > 0 && strings.Contains(s, sub))
}
