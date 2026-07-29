package contribution

import (
	"encoding/json"
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

	draft := &IssueDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
		ManifestID:    in.ManifestID,
	}
	populateDraftIdentity(&draft.DraftIdentity, in.Repo.String(), "issue", draft.Title, draft.Body, in.Evidence)
	draft.Warnings = append(draft.Warnings, ValidateRequiredTemplateSections([]byte(draft.Body), []byte(in.Guidance))...)
	return draft, nil
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

	r.writeValidationProof(&b, in.Evidence)
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

	draft := &PullRequestDraft{
		OpportunityID: o.ID,
		Title:         o.Title,
		Body:          strings.TrimSpace(b.String()),
		RenderedAt:    time.Now().UTC(),
		ManifestID:    in.ManifestID,
	}
	populateDraftIdentity(&draft.DraftIdentity, in.Repo.String(), "pull_request", draft.Title, draft.Body, in.Evidence)
	draft.Warnings = append(draft.Warnings, ValidateRequiredTemplateSections([]byte(draft.Body), []byte(in.Guidance))...)
	return draft, nil
}

func (r *Renderer) writeValidationProof(b *strings.Builder, all []*evidence.Evidence) {
	type runEvidence struct {
		item *evidence.Evidence
		run  *evidence.ValidationRun
		def  *evidence.ValidationDefinition
	}
	byDefinition := map[string]map[evidence.RunKind]runEvidence{}
	for _, item := range all {
		if item == nil || item.ValidationRun == nil || item.ValidationDefinition == nil {
			continue
		}
		run := item.ValidationRun
		if byDefinition[run.DefinitionID] == nil {
			byDefinition[run.DefinitionID] = map[evidence.RunKind]runEvidence{}
		}
		byDefinition[run.DefinitionID][run.Kind] = runEvidence{item: item, run: run, def: item.ValidationDefinition}
	}
	keys := make([]string, 0, len(byDefinition))
	for key := range byDefinition {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wroteHeading := false
	for _, key := range keys {
		runs := byDefinition[key]
		base, hasBase := runs[evidence.RunKindBase]
		candidate, hasCandidate := runs[evidence.RunKindCandidate]
		if !hasCandidate && !hasBase {
			continue
		}
		if !wroteHeading {
			b.WriteString("\n## Validation Proof\n\n")
			wroteHeading = true
		}
		command := formatArgv(firstDefinition(base, candidate).Command)
		if hasBase && hasCandidate {
			comparison, _ := evidence.Compare(base.run, candidate.run)
			if comparison.Classification == evidence.ComparisonFixed {
				fmt.Fprintf(b, "- **Before/after regression proof** (`%s`)\n", command)
				writeProofRun(b, "Base", base.run)
				writeProofRun(b, "Candidate", candidate.run)
				fmt.Fprintf(b, "  - Result: %s\n", comparison.Explanation)
				continue
			}
			fmt.Fprintf(b, "- **Inconclusive base/candidate comparison** (`%s`): %s\n", command, comparison.Explanation)
			writeProofRun(b, "Base", base.run)
			writeProofRun(b, "Candidate", candidate.run)
			continue
		}
		only := base
		label := "base"
		if hasCandidate {
			only, label = candidate, "candidate"
		}
		label = strings.ToUpper(label[:1]) + label[1:]
		fmt.Fprintf(b, "- **%s validation only** (`%s`): this is not causal regression proof.\n", label, command)
		writeProofRun(b, label, only.run)
	}
}

func firstDefinition(base, candidate struct {
	item *evidence.Evidence
	run  *evidence.ValidationRun
	def  *evidence.ValidationDefinition
}) *evidence.ValidationDefinition {
	if base.def != nil {
		return base.def
	}
	return candidate.def
}

func formatArgv(argv []string) string {
	if len(argv) == 0 {
		return "external command identity unavailable"
	}
	payload, err := json.Marshal(argv)
	if err != nil {
		return "command identity unavailable"
	}
	return boundedText(string(payload), 240) + " sha256:" + sha256Text(string(payload))
}

func writeProofRun(b *strings.Builder, label string, run *evidence.ValidationRun) {
	identity := run.WorkspaceSnapshotAfter
	if identity == "" {
		identity = run.WorkspaceSnapshotBefore
	}
	if run.ExecutionOrigin == "external" && run.External != nil {
		identity = run.External.Repository + "@" + run.External.Revision + " artifact " + run.External.ArtifactSHA256
	}
	fmt.Fprintf(b, "  - %s: %s (exit %d", label, run.Classification, run.ExitCode)
	if identity != "" {
		fmt.Fprintf(b, ", source `%s`", boundedText(identity, 160))
	}
	if run.ExecutionOrigin == "external" && run.External != nil {
		fmt.Fprintf(b, ", external receipt `%s` from %s", run.External.ReceiptSHA256, run.External.Producer)
	}
	b.WriteString(")\n")
	for _, observation := range run.Observations {
		if observation.Status == evidence.ObservationMatched && observation.Excerpt != "" {
			fmt.Fprintf(b, "    - %s: %s\n", observation.Name, boundedText(observation.Excerpt, 240))
		}
	}
}

func boundedText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
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
