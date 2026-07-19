package clustering

import (
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// ExtractMemberRefs adapts exact GitHub references to cluster member values.
func ExtractMemberRefs(text string, defaultRepo domain.RepoRef) []MemberRef {
	refs := similarity.ExtractRefs(text, defaultRepo)
	out := make([]MemberRef, len(refs))
	for i, ref := range refs {
		out[i] = MemberRef{Owner: ref.Repo.Owner, Repo: ref.Repo.Repo, Kind: string(ref.Kind), Number: ref.Number}
	}
	return out
}

func sameMemberRef(a, b MemberRef) bool {
	return a.Number == b.Number && a.Kind == b.Kind && strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Repo, b.Repo)
}
