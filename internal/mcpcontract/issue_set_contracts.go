package mcpcontract

// PrepareIssueSetInput selects an exact, bounded issue set for offline
// contribution-evidence preparation.
type PrepareIssueSetInput struct {
	Owner          string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo           string `json:"repo" jsonschema:"GitHub repository name"`
	IssueNumbers   []int  `json:"issue_numbers" jsonschema:"One to 20 distinct positive issue numbers"`
	PrecedentLimit int    `json:"precedent_limit,omitempty" jsonschema:"Maximum accepted examples per issue from 1 to 10"`
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"concise omits issue bodies and detailed relation evidence; detailed includes them"`
}

// IssueSetGap identifies evidence that is absent or incomplete and gives the
// exact bounded recovery call without treating the absence as negative proof.
type IssueSetGap struct {
	Code       string          `json:"code"`
	Facet      string          `json:"facet"`
	Status     string          `json:"status"`
	Message    string          `json:"message"`
	NextAction SuggestedAction `json:"next_action"`
}

// IssueSetRelatedWork is one corpus-supported issue, pull request, or external
// thread related to a supplied issue.
type IssueSetRelatedWork struct {
	Ref             string   `json:"ref"`
	Kind            string   `json:"kind"`
	Number          int      `json:"number"`
	Title           string   `json:"title,omitempty"`
	State           string   `json:"state,omitempty"`
	Relation        string   `json:"relation"`
	Direction       string   `json:"direction"`
	URL             string   `json:"url,omitempty"`
	Merged          *bool    `json:"merged,omitempty"`
	MergedAt        string   `json:"merged_at,omitempty"`
	SourceUpdatedAt string   `json:"source_updated_at,omitempty"`
	EvidenceKinds   []string `json:"evidence_kinds,omitempty"`
}

// IssueSetDuplicateCluster reports stored duplicate-candidate evidence for one
// supplied issue.
type IssueSetDuplicateCluster struct {
	StableID       string `json:"stable_id"`
	CanonicalRef   string `json:"canonical_ref"`
	CandidateCount int    `json:"candidate_count"`
}

// IssueSetLinkageCandidate is deliberately advisory. Callers must confirm the
// final PR-to-issue relationship after reviewing the actual implementation.
type IssueSetLinkageCandidate struct {
	IssueNumber          int      `json:"issue_number"`
	Relation             string   `json:"relation"`
	AllowedRelations     []string `json:"allowed_relations"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
	Basis                string   `json:"basis"`
}

// ContributionDisposition is a conservative, evidence-backed recommendation
// made before an implementation workspace is created.
type ContributionDisposition struct {
	Status       string   `json:"status"`
	Confidence   string   `json:"confidence"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Unknowns     []string `json:"unknowns,omitempty"`
	NextAction   string   `json:"next_action"`
}

// PreparedIssueEvidence contains stored facts and bounded derived evidence for
// one exact supplied issue.
type PreparedIssueEvidence struct {
	Number                  int                       `json:"number"`
	Title                   string                    `json:"title"`
	State                   string                    `json:"state"`
	StateReason             string                    `json:"state_reason,omitempty"`
	Labels                  []string                  `json:"labels,omitempty"`
	BodyStatus              string                    `json:"body_status"`
	Body                    string                    `json:"body,omitempty"`
	SourceUpdatedAt         string                    `json:"source_updated_at,omitempty"`
	Coverage                []FacetCoverageOutput     `json:"coverage"`
	Gaps                    []IssueSetGap             `json:"gaps,omitempty"`
	RelatedWork             []IssueSetRelatedWork     `json:"related_work"`
	RelatedWorkTotal        int                       `json:"related_work_total"`
	RelatedWorkTotalKnown   bool                      `json:"related_work_total_known"`
	RelatedWorkTruncated    bool                      `json:"related_work_truncated"`
	AcceptedExamples        []PrecedentOutput         `json:"accepted_examples"`
	DuplicateCluster        *IssueSetDuplicateCluster `json:"duplicate_cluster,omitempty"`
	Linkage                 IssueSetLinkageCandidate  `json:"linkage"`
	ContributionDisposition ContributionDisposition   `json:"contribution_disposition"`
}

// PrepareIssueSetOutput preserves requested issue order and reports all
// bounded-population and recovery information needed to interpret the result.
type PrepareIssueSetOutput struct {
	Status                 string                             `json:"status"`
	Owner                  string                             `json:"owner"`
	Repo                   string                             `json:"repo"`
	ResponseFormat         string                             `json:"response_format"`
	SourceAsOf             string                             `json:"source_as_of,omitempty"`
	Items                  []BatchItem[PreparedIssueEvidence] `json:"items"`
	Coverage               []FacetCoverageOutput              `json:"coverage"`
	Gaps                   []IssueSetGap                      `json:"gaps,omitempty"`
	RelationshipPopulation int                                `json:"relationship_population"`
	RelationshipConsidered int                                `json:"relationship_considered"`
	Truncated              bool                               `json:"truncated"`
	SuggestedActions       []SuggestedAction                  `json:"suggested_actions,omitempty"`
}
