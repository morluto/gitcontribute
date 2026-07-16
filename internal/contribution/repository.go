package contribution

import "context"

// Repository is a narrow persistence boundary for contribution drafts.
// Concrete stores live outside this package; production code never uses an
// in-memory store.
type Repository interface {
	SaveIssueDraft(ctx context.Context, d *IssueDraft) error
	GetIssueDraft(ctx context.Context, opportunityID string) (*IssueDraft, error)
	SavePullRequestDraft(ctx context.Context, d *PullRequestDraft) error
	GetPullRequestDraft(ctx context.Context, opportunityID string) (*PullRequestDraft, error)
}
