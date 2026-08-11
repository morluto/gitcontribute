package app

import (
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// parsePullRequestRefs canonicalizes identities and makes the optional kind
// explicit before duplicate detection. A blank kind means pull_request for
// pull-request-only operations.
func parsePullRequestRefs(inputs []mcpcontract.ThreadRef, path string) ([]mcpcontract.ThreadRef, error) {
	refs := append([]mcpcontract.ThreadRef(nil), inputs...)
	for i := range refs {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		ref, err := domain.NewRepoRef(refs[i].Owner, refs[i].Repo)
		if err != nil {
			return nil, mcpcontract.InvalidArgument(itemPath, err.Error(), nil)
		}
		if refs[i].Number <= 0 {
			return nil, mcpcontract.InvalidArgument(itemPath+".number", "must be positive", nil)
		}
		kind := strings.TrimSpace(refs[i].Kind)
		if kind == "" {
			kind = string(domain.PullRequestKind)
		}
		if kind != string(domain.PullRequestKind) {
			return nil, mcpcontract.InvalidArgument(itemPath+".kind", "must be pull_request when provided", nil)
		}
		refs[i] = mcpcontract.ThreadRef{Owner: ref.Owner(), Repo: ref.Repo(), Kind: kind, Number: refs[i].Number}
	}
	if err := rejectDuplicateThreadRefs(refs); err != nil {
		return nil, err
	}
	return refs, nil
}
