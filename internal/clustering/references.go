package clustering

import (
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

// ExtractMemberRefs adapts unquoted GitHub references to cluster member values.
func ExtractMemberRefs(text string, defaultRepo domain.RepoRef) []MemberRef {
	refs := relatedwork.Extract(text, defaultRepo)
	out := make([]MemberRef, len(refs))
	for i, ref := range refs {
		out[i] = MemberRef{Owner: ref.Repo.Owner(), Repo: ref.Repo.Repo(), Kind: ref.Kind, Number: ref.Number}
	}
	return out
}
