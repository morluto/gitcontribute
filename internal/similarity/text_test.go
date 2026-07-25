package similarity_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestTokensNormalizeAndFilterStopWords(t *testing.T) {
	got := similarity.Tokens("Hello, World! 123", true)
	want := []string{"123", "hello", "world"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("tokens mismatch (-want +got):\n%s", diff)
	}
	for _, token := range similarity.Tokens("the quick brown fox", true) {
		if token == "the" {
			t.Fatal("stop word found in tokens")
		}
	}
}

func TestExtractRefsRecognizesGitHubThreadSyntax(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"see #42 for context", 1},
		{"fix owner/repo#7", 1},
		{"https://github.com/owner/repo/issues/3", 1},
		{"see github.com/owner/repo/pull/4", 1},
		{"https://notgithub.com/owner/repo/issues/3", 0},
		{"nothing here", 0},
	} {
		if refs := similarity.ExtractRefs(tc.input, repo); len(refs) != tc.want {
			t.Fatalf("ExtractRefs(%q) = %d refs, want %d", tc.input, len(refs), tc.want)
		}
	}
	refs := similarity.ExtractRefs("see #42", repo)
	if len(refs) != 1 || refs[0].Number != 42 || refs[0].Kind != "" {
		t.Fatalf("bare ref mismatch: %+v", refs)
	}
}
