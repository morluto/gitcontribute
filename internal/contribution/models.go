package contribution

import (
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

// IssueDraft is a rendered issue body ready for review.
type IssueDraft struct {
	OpportunityID string
	DraftIdentity
	Title      string
	Body       string
	RenderedAt time.Time
	ManifestID string
}

// PullRequestDraft is a rendered PR body ready for review.
type PullRequestDraft struct {
	OpportunityID string
	DraftIdentity
	Title      string
	Body       string
	RenderedAt time.Time
	ManifestID string
}

// DraftIdentity binds one stored revision to the exact rendered UTF-8 bytes.
type DraftIdentity struct {
	ID          string
	Revision    int
	Repository  string
	Kind        string
	TitleBytes  int
	BodyBytes   int
	TitleSHA256 string
	BodySHA256  string
	EvidenceIDs []string
	Warnings    []DraftDiagnostic
}

// DraftDiagnostic reports an exact-byte validation finding without rewriting
// the draft.
type DraftDiagnostic struct {
	Code       string
	Severity   string
	Message    string
	ByteOffset int
}

// DraftArtifact is the common exact-byte view of an issue or pull-request
// draft revision.
type DraftArtifact struct {
	DraftIdentity
	OpportunityID string
	Title         string
	Body          string
	RenderedAt    time.Time
	ManifestID    string
}

// IssueInput supplies the verified facts and repository guidance used to render an issue.
type IssueInput struct {
	Opportunity *investigation.Opportunity
	Evidence    []*evidence.Evidence
	Guidance    string
	Repo        domain.RepoRef
	Success     string
	ManifestID  string
}

// PullRequestInput supplies the verified facts and repository guidance used to render a PR.
type PullRequestInput struct {
	Opportunity   *investigation.Opportunity
	Evidence      []*evidence.Evidence
	Guidance      string
	Repo          domain.RepoRef
	Approach      string
	Changes       string
	Compatibility string
	Limitations   string
	LinkedIssue   string
	ManifestID    string
}
