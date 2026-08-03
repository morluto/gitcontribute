package precedent

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestRepositoryKeyNormalizesOwnerAndRepository(t *testing.T) {
	got := RepositoryKey(domain.RepoRef{Owner: "Morluto", Repo: "GitContribute"})
	if got != "morluto/gitcontribute" {
		t.Fatalf("repository key = %q", got)
	}
}
