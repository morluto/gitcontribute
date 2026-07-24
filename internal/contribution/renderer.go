package contribution

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
)

// relationPriority gives a stable ordering for evidence sections.
var relationPriority = map[evidence.Relation]int{
	evidence.RelationSupporting:    0,
	evidence.RelationContradicting: 1,
	evidence.RelationInconclusive:  2,
	evidence.RelationStale:         3,
	evidence.RelationInvalid:       4,
}

// Renderer builds deterministic Markdown drafts from verified facts only.
type Renderer struct{}

// NewRenderer returns a deterministic contribution renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// RenderIssue builds an issue draft from the supplied opportunity, evidence,
// and repository guidance. It does not invent claims beyond the input.
func (r *Renderer) RenderIssue(in IssueInput) (*IssueDraft, error) {
	if in.Opportunity == nil {
		return nil, ErrMissingOpportunity
	}
	o := in.Opportunity
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "## Problem\n\n%s\n", o.ProblemStatement)
	if o.Scope != "" {
		_, _ = fmt.Fprintf(&b, "\n## Scope\n\n%s\n", o.Scope)
	}
	if o.Impact != "" {
		_, _ = fmt.Fprintf(&b, "\n## Impact\n\n%s\n", o.Impact)
	}

	r.writeEvidenceSection(&b, "Evidence", in.Evidence)
	r.writeReproductionSection(&b, in.Evidence)

	if in.Success != "" {
		fmt.Fprintf(&b, "\n## Success Criteria\n\n%s\n", in.Success)
	}
	if in.Guidance != "" {
		fmt.Fprintf(&b, "\n## Repository Guidance\n\n%s\n", in.Guidance)
	}

	return &IssueDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
		ManifestID:    in.ManifestID,
	}, nil
}

// RenderPullRequest builds a PR draft from the supplied opportunity, evidence,
// repository guidance, and explicit approach details.
func (r *Renderer) RenderPullRequest(in PullRequestInput) (*PullRequestDraft, error) {
	if in.Opportunity == nil {
		return nil, ErrMissingOpportunity
	}
	if in.Approach == "" {
		return nil, ErrMissingApproach
	}
	o := in.Opportunity
	var b strings.Builder

	fmt.Fprintf(&b, "## Motivation\n\n%s\n", o.ProblemStatement)
	if o.Impact != "" {
		fmt.Fprintf(&b, "\n## Concrete Outcome\n\n%s\n", o.Impact)
	}
	fmt.Fprintf(&b, "\n## Approach\n\n%s\n", in.Approach)
	if in.Changes != "" {
		fmt.Fprintf(&b, "\n## Focused Changes\n\n%s\n", in.Changes)
	}

	r.writeEvidenceSection(&b, "Validation", in.Evidence)

	if in.Compatibility != "" {
		fmt.Fprintf(&b, "\n## Compatibility\n\n%s\n", in.Compatibility)
	}
	if in.Limitations != "" {
		fmt.Fprintf(&b, "\n## Limitations\n\n%s\n", in.Limitations)
	}
	if in.LinkedIssue != "" {
		fmt.Fprintf(&b, "\n## Issue Linkage\n\n%s\n", in.LinkedIssue)
	}
	if in.Guidance != "" {
		fmt.Fprintf(&b, "\n## Repository Guidance\n\n%s\n", in.Guidance)
	}

	return &PullRequestDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
		ManifestID:    in.ManifestID,
	}, nil
}

func (r *Renderer) writeEvidenceSection(b *strings.Builder, heading string, all []*evidence.Evidence) {
	if len(all) == 0 {
		return
	}
	sorted := make([]*evidence.Evidence, 0, len(all))
	for _, item := range all {
		if item != nil {
			sorted = append(sorted, item)
		}
	}
	if len(sorted) == 0 {
		return
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, pj := relationPriority[sorted[i].Relation], relationPriority[sorted[j].Relation]
		if pi != pj {
			return pi < pj
		}
		return sorted[i].ID < sorted[j].ID
	})

	fmt.Fprintf(b, "\n## %s\n\n", heading)
	for _, e := range sorted {
		fmt.Fprintf(b, "- **%s**: %s", e.Relation, e.Description)
		if e.Type != "" {
			fmt.Fprintf(b, " (type: %s)", e.Type)
		}
		if e.ValidationRunID != "" {
			fmt.Fprintf(b, " [run: %s]", e.ValidationRunID)
		}
		b.WriteString("\n")
	}
}

func (r *Renderer) writeReproductionSection(b *strings.Builder, all []*evidence.Evidence) {
	var repros []*evidence.Evidence
	for _, e := range all {
		if e == nil {
			continue
		}
		if e.Type == evidence.EvidenceTypeMinimalReproduction ||
			e.Type == evidence.EvidenceTypeBaseFailingRegression {
			repros = append(repros, e)
		}
	}
	if len(repros) == 0 {
		return
	}
	sort.SliceStable(repros, func(i, j int) bool { return repros[i].ID < repros[j].ID })
	b.WriteString("\n## Reproduction\n\n")
	for _, e := range repros {
		fmt.Fprintf(b, "- %s", e.Description)
		if e.ValidationRunID != "" {
			fmt.Fprintf(b, " [run: %s]", e.ValidationRunID)
		}
		b.WriteString("\n")
	}
}
