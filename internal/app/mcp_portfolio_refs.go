package app

import (
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// canonicalPullRequestRefs makes the optional kind explicit before callers
// compare references. A blank kind means pull_request for portfolio operations,
// so it must not create a second identity for the same pull request.
func canonicalPullRequestRefs(inputs []mcpcontract.ThreadRef) []mcpcontract.ThreadRef {
	refs := append([]mcpcontract.ThreadRef(nil), inputs...)
	for i := range refs {
		if refs[i].Kind == "" {
			refs[i].Kind = corpus.ThreadKindPullRequest
		}
	}
	return refs
}
