package relatedwork

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestExtractClassifiesRelationshipsAndExcludesQuotedCode(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	refs := Extract("Fixes #1. Based on owner/repo#2. Blocks https://github.com/owner/repo/issues/3.\n> Fixes #4\n``Fixes #5``\n```\nFixes #6\n```", repo)
	if len(refs) != 3 {
		t.Fatalf("references = %+v", refs)
	}
	want := map[int]string{1: RelationClaimsToClose, 2: RelationDependsOn, 3: RelationBlocks}
	for _, ref := range refs {
		if ref.Repo != repo || want[ref.Number] != ref.Relation {
			t.Fatalf("reference = %+v, want relation %q", ref, want[ref.Number])
		}
	}
}

func TestExtractUsesStrongestRelationAndPreservesExplicitKind(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	refs := Extract("See https://github.com/owner/repo/pull/7; this depends on https://github.com/owner/repo/pull/7 and fixes https://github.com/owner/repo/pull/7.", repo)
	if len(refs) != 1 {
		t.Fatalf("references = %+v", refs)
	}
	for _, ref := range refs {
		if ref.Relation != RelationClaimsToClose {
			t.Fatalf("relation = %q, want %q", ref.Relation, RelationClaimsToClose)
		}
		if ref.Kind != domain.PullRequestKind || ref.Number != 7 {
			t.Fatalf("typed reference = %+v", ref)
		}
	}
}

func TestExtractRequiresMatchingFenceAndCodeSpanDelimiters(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	text := "```go\nFixes #1\n~~~\nFixes #2\n```\n" +
		"`Fixes\n#3`\n" +
		"`` Fixes #4 ` still code ``\n" +
		"Fixes #5"
	refs := Extract(text, repo)
	if len(refs) != 1 || refs[0].Number != 5 || refs[0].Relation != RelationClaimsToClose {
		t.Fatalf("references = %+v", refs)
	}
}

func TestExtractHandlesTildeFenceAndUnmatchedBacktickAsProse(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	refs := Extract("~~~text\nFixes #1\n~~~~\nAn unmatched ` does not hide Fixes #2", repo)
	if len(refs) != 1 || refs[0].Number != 2 || refs[0].Relation != RelationClaimsToClose {
		t.Fatalf("references = %+v", refs)
	}
}
