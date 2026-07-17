package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := io.Copy(w, &buf)
	return err
}

func humanOutput(v any) (string, error) {
	switch r := v.(type) {
	case *InitResult:
		return initHuman(r), nil
	case *StatusResult:
		return statusHuman(r), nil
	case *SyncResult:
		return syncHuman(r), nil
	case *SearchResult:
		return searchHuman(r), nil
	case *DossierResult:
		return dossierHuman(r), nil
	case *IndexResult:
		return fmt.Sprintf("Indexed %s at %s: %d files, %d bytes.\n%s", r.Repo, r.Commit, r.Files, r.Bytes, r.Message), nil
	case *SourceResult:
		return fmt.Sprintf("Source %s (%s): %s", r.Name, r.Kind, r.Definition), nil
	case *SourceListResult:
		var b strings.Builder
		fmt.Fprintf(&b, "%d sources", len(r.Sources))
		for _, source := range r.Sources {
			fmt.Fprintf(&b, "\n- %s (%s)", source.Name, source.Kind)
		}
		return b.String(), nil
	case *CrawlResult:
		return fmt.Sprintf("Crawled %s: %d repositories across %d windows using %d requests.\ncheckpoint: %s", r.Source, r.Repositories, r.Windows, r.Requests, r.Checkpoint), nil
	case *InvestigationResult:
		return investigationHuman(r), nil
	case *InvestigationListResult:
		return investigationListHuman(r), nil
	case *HypothesisResult:
		return hypothesisHuman(r), nil
	case *HypothesisListResult:
		return hypothesisListHuman(r), nil
	case *OpportunityResult:
		return opportunityHuman(r), nil
	case *OpportunityListResult:
		return opportunityListHuman(r), nil
	case *WorkspaceResult:
		return workspaceHuman(r), nil
	case *ValidationResult:
		return validationHuman(r), nil
	case *ValidationRunResult:
		return validationRunHuman(r), nil
	case *ValidationComparisonResult:
		return validationComparisonHuman(r), nil
	case *EvidenceResult:
		return evidenceHuman(r), nil
	case *DraftResult:
		return draftHuman(r), nil
	default:
		return "", fmt.Errorf("unsupported result type %T", v)
	}
}

func initHuman(r *InitResult) string {
	return fmt.Sprintf("Initialized corpus at %s.\n%s", r.Path, r.Message)
}

func statusHuman(r *StatusResult) string {
	state := "not healthy"
	if r.Healthy {
		state = "healthy"
	}
	return fmt.Sprintf("Status: %s (corpus=%s version=%s)\n%s", state, r.Corpus, r.Version, r.Message)
}

func syncHuman(r *SyncResult) string {
	return fmt.Sprintf("Synced %s: %d updated.\n%s", r.Repo, r.Updated, r.Message)
}

func searchHuman(r *SearchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Search: %s (kind=%s, limit=%d)", r.Query, r.Kind, r.Limit)
	if r.Repo != "" {
		fmt.Fprintf(&b, "\nrepo filter: %s", r.Repo)
	}
	if len(r.Matches) == 0 {
		b.WriteString("\nNo matches found.")
		return b.String()
	}
	fmt.Fprintf(&b, "\n%d matches:", r.Total)
	for _, m := range r.Matches {
		fmt.Fprintf(&b, "\n- %s %s", m.Kind, m.Repo)
		if m.Number > 0 {
			fmt.Fprintf(&b, "#%d", m.Number)
		}
		fmt.Fprintf(&b, ": %s", m.Title)
		if m.Score != 0 {
			fmt.Fprintf(&b, " (%.2f)", m.Score)
		}
	}
	return b.String()
}

func dossierHuman(r *DossierResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Dossier: %s\n", r.Repo)
	fmt.Fprintf(&b, "Summary: %s\n", r.Summary)
	fmt.Fprintf(&b, "Language: %s\n", r.Language)
	fmt.Fprintf(&b, "Stars: %d\n", r.Stars)
	fmt.Fprintf(&b, "Open issues: %d\n", r.OpenIssues)
	fmt.Fprintf(&b, "Coverage: %s\n", strings.Join(r.Coverage, ", "))
	fmt.Fprintf(&b, "Freshness: %s", r.Freshness)
	return b.String()
}

func investigationHuman(r *InvestigationResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Investigation: %s (%s)\n", r.ID, r.Status)
	fmt.Fprintf(&b, "Repository: %s", r.Repo)
	if r.CommitSHA != "" {
		fmt.Fprintf(&b, " @ %s", r.CommitSHA)
	}
	if r.Lens != "" {
		fmt.Fprintf(&b, " lens=%s", r.Lens)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Created: %s\nUpdated: %s", r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func investigationListHuman(r *InvestigationListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s", len(r.Investigations), pluralize(len(r.Investigations), "investigation", "investigations"))
	for _, inv := range r.Investigations {
		fmt.Fprintf(&b, "\n- %s %s (%s)", inv.ID, inv.Repo, inv.Status)
	}
	return b.String()
}

func hypothesisHuman(r *HypothesisResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hypothesis: %s (%s)\n", r.ID, r.Status)
	fmt.Fprintf(&b, "Investigation: %s\n", r.InvestigationID)
	fmt.Fprintf(&b, "Category: %s\n", r.Category)
	fmt.Fprintf(&b, "Title: %s\n", r.Title)
	fmt.Fprintf(&b, "Description: %s\n", r.Description)
	fmt.Fprintf(&b, "Created: %s\nUpdated: %s", r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func hypothesisListHuman(r *HypothesisListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s", len(r.Hypotheses), pluralize(len(r.Hypotheses), "hypothesis", "hypotheses"))
	for _, h := range r.Hypotheses {
		fmt.Fprintf(&b, "\n- %s [%s] %s (%s)", h.ID, h.Category, h.Title, h.Status)
	}
	return b.String()
}

func opportunityHuman(r *OpportunityResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Opportunity: %s (%s)\n", r.ID, r.Status)
	fmt.Fprintf(&b, "Investigation: %s\n", r.InvestigationID)
	fmt.Fprintf(&b, "Hypothesis: %s\n", r.HypothesisID)
	fmt.Fprintf(&b, "Title: %s\n", r.Title)
	fmt.Fprintf(&b, "Problem: %s\n", r.ProblemStatement)
	fmt.Fprintf(&b, "Scope: %s\n", r.Scope)
	fmt.Fprintf(&b, "Impact: %s\n", r.Impact)
	fmt.Fprintf(&b, "Effort: %s\n", r.ExpectedEffort)
	fmt.Fprintf(&b, "Confidence: %.2f\n", r.Confidence)
	fmt.Fprintf(&b, "Category: %s\n", r.Category)
	fmt.Fprintf(&b, "Collisions: %s\n", r.CollisionStatus)
	fmt.Fprintf(&b, "Created: %s\nUpdated: %s", r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func opportunityListHuman(r *OpportunityListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s", len(r.Opportunities), pluralize(len(r.Opportunities), "opportunity", "opportunities"))
	if r.Filter != "" {
		fmt.Fprintf(&b, " (filter: %s)", r.Filter)
	}
	for _, o := range r.Opportunities {
		fmt.Fprintf(&b, "\n- %s [%s] %s (confidence %.2f, %s)", o.ID, o.Category, o.Title, o.Confidence, o.Status)
	}
	return b.String()
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func workspaceHuman(r *WorkspaceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workspace: %s (investigation=%s, repo=%s, dirty=%v)\n", r.ID, r.InvestigationID, r.Repo, r.Dirty)
	fmt.Fprintf(&b, "Path: %s\n", r.Path)
	fmt.Fprintf(&b, "Remote: %s\n", r.Remote)
	fmt.Fprintf(&b, "Base SHA: %s\n", r.BaseSHA)
	fmt.Fprintf(&b, "Candidate SHA: %s\n", r.CandidateSHA)
	fmt.Fprintf(&b, "Merge base: %s\n", r.MergeBase)
	fmt.Fprintf(&b, "Created: %s", r.CreatedAt)
	return b.String()
}

func validationHuman(r *ValidationResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation: %s (kind=%s, investigation=%s)\n", r.ID, r.Kind, r.InvestigationID)
	fmt.Fprintf(&b, "Command: %s\n", strings.Join(r.Command, " "))
	fmt.Fprintf(&b, "Working directory: %s\n", r.WorkingDir)
	if r.BaseWorkingDir != "" {
		fmt.Fprintf(&b, "Base working directory: %s\n", r.BaseWorkingDir)
	}
	if r.CandidateDir != "" {
		fmt.Fprintf(&b, "Candidate directory: %s\n", r.CandidateDir)
	}
	if r.Timeout != "" {
		fmt.Fprintf(&b, "Timeout: %s\n", r.Timeout)
	}
	if r.MaxOutputBytes > 0 {
		fmt.Fprintf(&b, "Max output bytes: %d\n", r.MaxOutputBytes)
	}
	if len(r.Env) > 0 {
		fmt.Fprintf(&b, "Environment allowlist: %s\n", strings.Join(r.Env, ", "))
	}
	fmt.Fprintf(&b, "Created: %s", r.CreatedAt)
	return b.String()
}

func validationRunHuman(r *ValidationRunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation run: %s (kind=%s, classification=%s, exit=%d)\n", r.ID, r.Kind, r.Classification, r.ExitCode)
	if r.Truncated {
		b.WriteString("Output truncated\n")
	}
	if r.Stdout != "" {
		fmt.Fprintf(&b, "--- stdout ---\n%s\n", r.Stdout)
	}
	if r.Stderr != "" {
		fmt.Fprintf(&b, "--- stderr ---\n%s\n", r.Stderr)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", r.Error)
	}
	fmt.Fprintf(&b, "Started: %s\n", r.StartedAt)
	fmt.Fprintf(&b, "Completed: %s", r.CompletedAt)
	return b.String()
}

func validationComparisonHuman(r *ValidationComparisonResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Comparison: %s\n", r.Classification)
	fmt.Fprintf(&b, "Explanation: %s\n", r.Explanation)
	if r.Base != nil {
		fmt.Fprintf(&b, "Base run: %s (exit=%d, %s)\n", r.Base.ID, r.Base.ExitCode, r.Base.Classification)
	}
	if r.Candidate != nil {
		fmt.Fprintf(&b, "Candidate run: %s (exit=%d, %s)\n", r.Candidate.ID, r.Candidate.ExitCode, r.Candidate.Classification)
	}
	return b.String()
}

func evidenceHuman(r *EvidenceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Evidence for investigation %s:\n", r.InvestigationID)
	if len(r.Evidence) == 0 {
		b.WriteString("No evidence recorded.")
		return b.String()
	}
	for _, e := range r.Evidence {
		fmt.Fprintf(&b, "- %s [%s / %s] %s", e.ID, e.Type, e.Relation, e.Description)
		if e.ValidationRunID != "" {
			fmt.Fprintf(&b, " [run: %s]", e.ValidationRunID)
		}
		if e.OpportunityID != "" {
			fmt.Fprintf(&b, " [opportunity: %s]", e.OpportunityID)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func draftHuman(r *DraftResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s draft for opportunity %s\n", r.Kind, r.OpportunityID)
	fmt.Fprintf(&b, "Title: %s\n", r.Title)
	fmt.Fprintf(&b, "--- Body ---\n%s\n--- End ---\n", r.Body)
	fmt.Fprintf(&b, "Rendered: %s", r.RenderedAt)
	return b.String()
}
