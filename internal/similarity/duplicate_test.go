package similarity_test

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestDuplicateV1ExplainsExplicitReference(t *testing.T) {
	rule := similarity.DefaultDuplicateRule()
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	a := rule.Prepare(similarity.ThreadText{
		Ref:   similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 1},
		Title: "bug",
		Body:  "first",
	})
	b := rule.Prepare(similarity.ThreadText{
		Ref:   similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 2},
		Title: "other",
		Body:  "duplicate of #1",
	})

	got := rule.Compare(a, b)
	if got.Value != 0.40 {
		t.Fatalf("score = %.2f, want 0.40", got.Value)
	}
	if got.Reason != "explicit reference" {
		t.Fatalf("reason = %q, want explicit reference", got.Reason)
	}
	if got.RuleVersion != similarity.DuplicateV1 {
		t.Fatalf("rule version = %q, want %q", got.RuleVersion, similarity.DuplicateV1)
	}
}
