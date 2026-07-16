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

	b.WriteString(fmt.Sprintf("## Problem\n\n%s\n", o.ProblemStatement))
	if o.Scope != "" {
		b.WriteString(fmt.Sprintf("\n## Scope\n\n%s\n", o.Scope))
	}
	if o.Impact != "" {
		b.WriteString(fmt.Sprintf("\n## Impact\n\n%s\n", o.Impact))
	}

	r.writeEvidenceSection(&b, "Evidence", in.Evidence)
	r.writeReproductionSection(&b, in.Evidence)

	if in.Success != "" {
		b.WriteString(fmt.Sprintf("\n## Success Criteria\n\n%s\n", in.Success))
	}
	if in.Guidance != "" {
		b.WriteString(fmt.Sprintf("\n## Repository Guidance\n\n%s\n", in.Guidance))
	}

	return &IssueDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
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

	b.WriteString(fmt.Sprintf("## Motivation\n\n%s\n", o.ProblemStatement))
	if o.Impact != "" {
		b.WriteString(fmt.Sprintf("\n## Concrete Outcome\n\n%s\n", o.Impact))
	}
	b.WriteString(fmt.Sprintf("\n## Approach\n\n%s\n", in.Approach))
	if in.Changes != "" {
		b.WriteString(fmt.Sprintf("\n## Focused Changes\n\n%s\n", in.Changes))
	}

	r.writeEvidenceSection(&b, "Validation", in.Evidence)

	if in.Compatibility != "" {
		b.WriteString(fmt.Sprintf("\n## Compatibility\n\n%s\n", in.Compatibility))
	}
	if in.Limitations != "" {
		b.WriteString(fmt.Sprintf("\n## Limitations\n\n%s\n", in.Limitations))
	}
	if in.LinkedIssue != "" {
		b.WriteString(fmt.Sprintf("\n## Issue Linkage\n\n%s\n", in.LinkedIssue))
	}
	if in.Guidance != "" {
		b.WriteString(fmt.Sprintf("\n## Repository Guidance\n\n%s\n", in.Guidance))
	}

	return &PullRequestDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
	}, nil
}

func (r *Renderer) writeEvidenceSection(b *strings.Builder, heading string, all []*evidence.Evidence) {
	if len(all) == 0 {
		return
	}
	sorted := make([]*evidence.Evidence, len(all))
	copy(sorted, all)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, pj := relationPriority[sorted[i].Relation], relationPriority[sorted[j].Relation]
		if pi != pj {
			return pi < pj
		}
		return sorted[i].ID < sorted[j].ID
	})

	b.WriteString(fmt.Sprintf("\n## %s\n\n", heading))
	for _, e := range sorted {
		b.WriteString(fmt.Sprintf("- **%s**: %s", e.Relation, e.Description))
		if e.Type != "" {
			b.WriteString(fmt.Sprintf(" (type: %s)", e.Type))
		}
		if e.ValidationRunID != "" {
			b.WriteString(fmt.Sprintf(" [run: %s]", e.ValidationRunID))
		}
		b.WriteString("\n")
	}
}

func (r *Renderer) writeReproductionSection(b *strings.Builder, all []*evidence.Evidence) {
	var repros []*evidence.Evidence
	for _, e := range all {
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
		b.WriteString(fmt.Sprintf("- %s", e.Description))
		if e.ValidationRunID != "" {
			b.WriteString(fmt.Sprintf(" [run: %s]", e.ValidationRunID))
		}
		b.WriteString("\n")
	}
}
