package precedent

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestRepositoryKeyNormalizesOwnerAndRepository(t *testing.T) {
	got := RepositoryKey(domain.MustRepoRef("Morluto", "GitContribute"))
	if got != "morluto/gitcontribute" {
		t.Fatalf("repository key = %q", got)
	}
}
