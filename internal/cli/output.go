package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/radar"
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
	case *MetadataResult:
		return metadataHuman(r), nil
	case *ConfigureResult:
		return configureHuman(r), nil
	case *ControlStatusResult:
		return controlStatusHuman(r), nil
	case *DoctorResult:
		return doctorHuman(r), nil
	case *CorpusInspectionResult:
		return corpusInspectionHuman(r), nil
	case *CorpusBackupResult:
		result := fmt.Sprintf("Backed up corpus to %s (%d bytes, sha256 %s)", r.Path, r.SizeBytes, r.SHA256)
		if r.ManifestPath != "" {
			result += fmt.Sprintf("\nManifest: %s (schema %d, expected %d, %s)", r.ManifestPath, r.SourceSchema, r.ExpectedSchema, r.Compatibility)
		}
		return result, nil
	case *CorpusMigrationResult:
		return corpusMigrationHuman(r), nil
	case *CorpusRestoreResult:
		return corpusRestoreHuman(r), nil
	case *CorpusInventoryResult:
		return corpusInventoryHuman(r), nil
	case *CorpusInventoryListResult:
		return corpusInventoryListHuman(r), nil
	case *CorpusPruneResult:
		return corpusPruneHuman(r), nil
	case *CorpusRepositoryRemovalResult:
		return corpusRepositoryRemovalHuman(r), nil
	case *CorpusProjectionListResult:
		return corpusProjectionListHuman(r), nil
	case *CorpusProjectionResult:
		return corpusProjectionHuman("Rebuilt projection", r), nil
	case *SyncResult:
		return syncHuman(r), nil
	case *radar.Report:
		return radarHuman(r), nil
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
		return crawlHuman(r), nil
	case *InvestigationResult:
		return investigationHuman(r), nil
	case *ThreadInvestigationResult:
		return threadInvestigationHuman(r), nil
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
	case *ReadinessResult:
		return readinessHuman(r), nil
	case *ReadinessCheck:
		return readinessCheckHuman(r), nil
	case *DraftResult:
		return draftHuman(r), nil
	case *ClusterListResult:
		return clusterListHuman(r), nil
	case *ClusterRefreshResult:
		return fmt.Sprintf("cluster projection %s for %s (%d candidates, %d comparisons, rule %s)", r.Disposition, r.Repo, r.Stats.CandidateCount, r.Stats.ComparedPairs, r.Projection.RuleVersion), nil
	case *ClusterResult:
		return clusterHuman(r), nil
	case *LensResult:
		return lensHuman(r), nil
	case *LensListResult:
		return lensListHuman(r), nil
	case *LensExplainResult:
		return lensExplainHuman(r), nil
	case *CollectionResult:
		return collectionHuman(r), nil
	case *CollectionListResult:
		return collectionListHuman(r), nil
	case *HydrateResult:
		return hydrateHuman(r), nil
	case *CoverageResult:
		return coverageHuman(r), nil
	case *RunListResult:
		return runsHuman(r), nil
	case *JobListResult:
		return jobsHuman(r), nil
	case *JobResult:
		return jobHuman(r), nil
	case *NeighborListResult:
		return neighborsHuman(r), nil
	case *TriageEventResult:
		return triageEventHuman(r), nil
	case *TriageEventListResult:
		return triageEventListHuman(r), nil
	case *ContributionResult:
		return contributionHuman(r), nil
	case *ContributionListResult:
		return contributionListHuman(r), nil
	case *ContributionOutcomeResult:
		return contributionOutcomeHuman(r), nil
	case *ContributionOutcomeListResult:
		return contributionOutcomeListHuman(r), nil
	case *MetadataImportResult:
		return metadataImportHuman(r), nil
	default:
		payload, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", fmt.Errorf("render %T: %w", v, err)
		}
		return string(payload), nil
	}
}

func corpusProjectionListHuman(r *CorpusProjectionListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Corpus projections: %d", len(r.Projections))
	for _, projection := range r.Projections {
		fmt.Fprintf(&b, "\n- %s", corpusProjectionHuman("", &projection))
	}
	return b.String()
}

func corpusProjectionHuman(prefix string, r *CorpusProjectionResult) string {
	line := fmt.Sprintf("%s %s: %s, %d rows", r.Name, r.Version, r.Status, r.RowCount)
	if prefix != "" {
		line = fmt.Sprintf("%s %s at %s: %s, %d rows", prefix, r.Name, r.Version, r.Status, r.RowCount)
	}
	if r.SourceRevision != "" {
		line += "; source " + r.SourceRevision
	}
	if r.AttemptStatus != "" {
		line += "; last build " + r.AttemptStatus
	}
	if r.AttemptError != "" {
		line += ": " + oneLine(r.AttemptError)
	}
	return line
}

func corpusInventoryHuman(r *CorpusInventoryResult) string {
	return fmt.Sprintf("Corpus inventory %s: %d issues, %d pull requests, %d observations; %d code snapshots, %d code bytes; database %d bytes + WAL %d bytes",
		r.Repo, r.Issues, r.PullRequests, r.RepositoryObservations+r.ThreadObservations+r.FacetObservations, r.CodeSnapshots, r.CodeBytes, r.DatabaseBytes, r.WALBytes)
}

func corpusInventoryListHuman(r *CorpusInventoryListResult) string {
	var b strings.Builder
	state := "unknown"
	if r.Schema != nil {
		state = fmt.Sprintf("%s (schema %d/%d)", r.Schema.State, r.Schema.Current, r.Schema.Target)
	}
	fmt.Fprintf(&b, "Corpus inventory: %d repository scopes; %s", len(r.Repositories), state)
	fmt.Fprintf(&b, "\nStorage: database %d bytes + WAL %d bytes; observation payloads %d bytes; code content %d bytes", r.DatabaseBytes, r.WALBytes, r.ObservationPayloadBytes, r.CodeBytes)
	for _, repo := range r.Repositories {
		observations := repo.RepositoryObservations + repo.ThreadObservations + repo.FacetObservations
		fmt.Fprintf(&b, "\n- %s: %d issues, %d pull requests, %d observations, %d code snapshots", repo.Repo, repo.Issues, repo.PullRequests, observations, repo.CodeSnapshots)
		if repo.LatestObservationAt != "" {
			fmt.Fprintf(&b, "; latest observation %s", repo.LatestObservationAt)
		}
	}
	if len(r.PendingWork) == 0 {
		b.WriteString("\nPending work: none")
	} else {
		fmt.Fprintf(&b, "\nPending work: %d", len(r.PendingWork))
		for _, work := range r.PendingWork {
			fmt.Fprintf(&b, "\n- %s %s: %s", work.Kind, work.Name, work.Status)
			if work.Detail != "" {
				fmt.Fprintf(&b, " (%s)", oneLine(work.Detail))
			}
		}
	}
	return b.String()
}

func corpusPruneHuman(r *CorpusPruneResult) string {
	action := "Would delete"
	if !r.DryRun {
		action = "Deleted"
	}
	return fmt.Sprintf("%s %d derived code snapshots for %s; keep latest %d; reclaimable bytes %d", action, len(r.Delete), r.Repo, r.KeepLatest, r.ReclaimBytes)
}

func corpusRepositoryRemovalHuman(r *CorpusRepositoryRemovalResult) string {
	action := "Would remove"
	if !r.DryRun {
		action = "Removed"
	}
	linkedRecords := r.DetachedTriageEvents + r.RemovedPortfolioLinks + r.RemovedResolutionRecords + r.RemovedSignalSnapshots + r.DetachedClusterMembers
	return fmt.Sprintf("%s repository %s: %d threads, %d observations, %d code snapshots, %d dossiers, %d clusters, %d linked records; preserved %d investigations and %d cross-repository references",
		action, r.Repo, r.Threads, r.RepositoryObservations+r.ThreadObservations+r.FacetObservations,
		r.CodeSnapshots, r.Dossiers, r.Clusters, linkedRecords, r.PreservedInvestigations, r.PreservedCrossRepoReferences)
}

func corpusRestoreHuman(r *CorpusRestoreResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restored corpus from %s", r.Source)
	if r.SafetyBackup != nil {
		fmt.Fprintf(&b, "\nSafety backup: %s (sha256 %s)", r.SafetyBackup.Path, r.SafetyBackup.SHA256)
		if r.SafetyBackup.ManifestPath != "" {
			fmt.Fprintf(&b, "\nSafety backup manifest: %s", r.SafetyBackup.ManifestPath)
		}
	}
	if r.After != nil {
		fmt.Fprintf(&b, "\nSchema %d; %d repositories, %d threads", r.After.Current, r.After.Repositories, r.After.Threads)
	}
	return b.String()
}

func corpusInspectionHuman(r *CorpusInspectionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Corpus %s: %s (schema %d -> %d, %d bytes)", r.Path, r.State, r.Current, r.Target, r.SizeBytes)
	fmt.Fprintf(&b, "\n%d repositories, %d threads", r.Repositories, r.Threads)
	for _, step := range r.Pending {
		fmt.Fprintf(&b, "\n- migration %03d: %s", step.Version, step.Name)
	}
	return b.String()
}

func corpusMigrationHuman(r *CorpusMigrationResult) string {
	var b strings.Builder
	if r.Before != nil && r.After != nil {
		fmt.Fprintf(&b, "Migrated corpus schema %d -> %d", r.Before.Current, r.After.Current)
	} else {
		b.WriteString("Corpus migration completed")
	}
	if r.Backup != nil {
		fmt.Fprintf(&b, "\nBackup: %s (sha256 %s)", r.Backup.Path, r.Backup.SHA256)
		if r.Backup.ManifestPath != "" {
			fmt.Fprintf(&b, "\nBackup manifest: %s (schema %d, expected %d, %s)", r.Backup.ManifestPath, r.Backup.SourceSchema, r.Backup.ExpectedSchema, r.Backup.Compatibility)
		}
	}
	for _, step := range r.Steps {
		if step.Phase == "completed" {
			fmt.Fprintf(&b, "\n- migration %03d completed: %s", step.Version, step.Name)
		}
	}
	return b.String()
}

func radarHuman(r *radar.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Contribution radar: %s (%s)", r.Repo, r.ScoreVersion)
	fmt.Fprintf(&b, "\nRanked %d of %d open issues from a %d-issue population", len(r.Candidates), r.TotalOpenIssues, r.CandidatePopulation)
	if r.PopulationCapped {
		b.WriteString(" (bounded)")
	}
	if !r.SourceAsOf.IsZero() {
		fmt.Fprintf(&b, "\nSource evidence as of %s", r.SourceAsOf.UTC().Format("2006-01-02T15:04:05Z"))
	}
	for _, candidate := range r.Candidates {
		fmt.Fprintf(&b, "\n\n%d. [%s] #%d %s — %d/100, base %d (%s confidence)",
			candidate.Rank, candidate.Eligibility, candidate.Number, oneLine(candidate.Title), candidate.Score, candidate.BaseScore, candidate.Confidence)
		writeRadarSignals(&b, "why", candidate.PositiveSignals)
		writeRadarSignals(&b, "risks", candidate.Risks)
		writeRadarSignals(&b, "blockers", candidate.Blockers)
		writeRadarRelatedWork(&b, candidate.RelatedWork)
		if len(candidate.Unknowns) > 0 {
			fmt.Fprintf(&b, "\n   unknown: %s", joinRadarUnknowns(candidate.Unknowns))
		}
		if candidate.URL != "" {
			fmt.Fprintf(&b, "\n   %s", candidate.URL)
		}
	}
	if len(r.Unknowns) > 0 {
		fmt.Fprintf(&b, "\n\nRepository unknowns: %s", joinRadarUnknowns(r.Unknowns))
	}
	return b.String()
}

func writeRadarRelatedWork(b *strings.Builder, values []radar.RelatedWork) {
	if len(values) == 0 {
		return
	}
	const displayLimit = 5
	limit := min(len(values), displayLimit)
	parts := make([]string, 0, limit+1)
	for _, value := range values[:limit] {
		part := value.Relation + " " + value.Ref
		if value.State != "" {
			part += " [" + value.State + "]"
		}
		parts = append(parts, part)
	}
	if len(values) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(values)-limit))
	}
	fmt.Fprintf(b, "\n   related: %s", strings.Join(parts, "; "))
}

func writeRadarSignals(b *strings.Builder, label string, signals []radar.Signal) {
	if len(signals) == 0 {
		return
	}
	parts := make([]string, 0, len(signals))
	for _, signal := range signals {
		part := signal.Summary
		if signal.Weight > 0 {
			part += fmt.Sprintf(" (+%d)", signal.Weight)
		} else if signal.Weight < 0 {
			part += fmt.Sprintf(" (%d)", signal.Weight)
		}
		parts = append(parts, part)
	}
	fmt.Fprintf(b, "\n   %s: %s", label, strings.Join(parts, "; "))
}

func joinRadarUnknowns(unknowns []radar.Unknown) string {
	parts := make([]string, 0, len(unknowns))
	for _, unknown := range unknowns {
		parts = append(parts, unknown.Summary)
	}
	return strings.Join(parts, "; ")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func metadataHuman(r *MetadataResult) string {
	return fmt.Sprintf("%s %s (%s/%s, %s)\nSchema: %d\nCorpus: %s\nCapabilities: %s",
		r.Name, r.Version, r.OS, r.Architecture, r.GoVersion, r.SchemaVersion, r.CorpusPath, strings.Join(r.Capabilities, ", "))
}

func configureHuman(r *ConfigureResult) string {
	action := "Configuration unchanged"
	if r.Changed {
		action = "Configuration updated"
	}
	if r.DryRun {
		action = "Configuration dry run"
	}
	return fmt.Sprintf("%s: %s\nDatabase: %s\nToken source: %s\nCrawl: budget=%d concurrency=%d retries=%d timeout=%s\nOutput: %s, max=%d",
		action, r.Path, r.Config.Database, r.Config.TokenSource, r.Config.CrawlBudget,
		r.Config.CrawlConcurrency, r.Config.CrawlRetryLimit, r.Config.CrawlTimeout,
		r.Config.OutputFormat, r.Config.OutputMaxResults)
}

func controlStatusHuman(r *ControlStatusResult) string {
	state := "healthy"
	if !r.Healthy {
		state = "not healthy"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Status: %s (corpus=%s version=%s schema=%d)", state, r.Corpus, r.Version, r.SchemaVersion)
	fmt.Fprintf(&b, "\n%d repositories, %d threads, %d sources; %d ready frontier items, %d active runs, %d active jobs",
		r.Counts.Repositories, r.Counts.Threads, r.Counts.Sources, r.Counts.FrontierReady, r.Counts.ActiveRuns, r.Counts.ActiveJobs)
	if r.FreshestSource != "" {
		fmt.Fprintf(&b, "\nFreshest source: %s", r.FreshestSource)
	}
	for _, rate := range r.RateLimits {
		fmt.Fprintf(&b, "\nGitHub rate limit %s: %d/%d remaining (observed %s)",
			rate.Resource, rate.Remaining, rate.Limit, rate.ObservedAt)
		if rate.ResetAt != "" {
			fmt.Fprintf(&b, ", resets %s", rate.ResetAt)
		}
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s", warning)
	}
	return b.String()
}

func doctorHuman(r *DoctorResult) string {
	state := "healthy"
	if !r.Healthy {
		state = "unhealthy"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Doctor: %s", state)
	for _, check := range r.Checks {
		fmt.Fprintf(&b, "\n- %s [%s]: %s", check.Name, check.Status, check.Message)
	}
	return b.String()
}

func jobsHuman(r *JobListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d jobs", len(r.Jobs))
	for i := range r.Jobs {
		fmt.Fprintf(&b, "\n- %s", jobHuman(&r.Jobs[i]))
	}
	return b.String()
}

func jobHuman(r *JobResult) string {
	line := fmt.Sprintf("%s %s [%s] created %s", r.ID, r.Kind, r.Status, r.CreatedAt)
	if r.Progress != "" {
		line += ": " + r.Progress
	}
	if r.Error != "" {
		line += ": " + r.Error
	}
	return line
}

func hydrateHuman(r *HydrateResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hydrated %s#%d (%s): %d requests", r.Repo, r.Number, r.Kind, r.Requests)
	for _, facet := range r.Facets {
		status := "partial"
		if facet.Complete {
			status = "complete"
		}
		fmt.Fprintf(&b, "\n- %s: %d records across %d pages (%s)", facet.Facet, facet.Count, facet.Pages, status)
	}
	return b.String()
}

func coverageHuman(r *CoverageResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Coverage for %s:", r.Repo)
	if len(r.Facets) == 0 {
		b.WriteString(" none")
		return b.String()
	}
	for _, facet := range r.Facets {
		status := "partial"
		if facet.Complete {
			status = "complete"
		}
		fmt.Fprintf(&b, "\n- %s: %s", facet.Facet, status)
		if facet.UpdatedAt != "" {
			fmt.Fprintf(&b, " (updated %s)", facet.UpdatedAt)
		}
	}
	return b.String()
}

func runsHuman(r *RunListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d runs", len(r.Runs))
	for _, run := range r.Runs {
		fmt.Fprintf(&b, "\n- %d %s [%s] started %s", run.ID, run.Kind, run.Status, run.StartedAt)
		if run.Error != "" {
			fmt.Fprintf(&b, ": %s", run.Error)
		}
	}
	return b.String()
}

func neighborsHuman(r *NeighborListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Neighbors for %s:%s#%d (revision %s)", r.Repo, r.Kind, r.Number, r.SourceRevision)
	if len(r.Neighbors) == 0 {
		b.WriteString("\nNo neighbors found.")
		return b.String()
	}
	for _, neighbor := range r.Neighbors {
		fmt.Fprintf(&b, "\n- %s:%s#%d %.2f: %s — %s", neighbor.Repo, neighbor.Kind, neighbor.Number, neighbor.Score, neighbor.Title, neighbor.Reason)
	}
	return b.String()
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
	planning := ""
	if r.PlannedRequests > 0 {
		planning = fmt.Sprintf(" Requests: %d actual, up to %d planned (budget %d).", r.Requests, r.PlannedRequests, r.RequestBudget)
	}
	return fmt.Sprintf("Synced %s: %d updated.%s\n%s", r.Repo, r.Updated, planning, r.Message)
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

func crawlHuman(r *CrawlResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Crawled %s: %d repositories", r.Source, r.Repositories)
	if r.Threads > 0 {
		fmt.Fprintf(&b, ", %d threads", r.Threads)
	}
	if r.Events > 0 {
		fmt.Fprintf(&b, ", %d events", r.Events)
	}
	fmt.Fprintf(&b, " across %d windows using %d requests", r.Windows, r.Requests)
	if r.Imported > 0 || r.Skipped > 0 || r.Failures > 0 {
		fmt.Fprintf(&b, " (imported %d, skipped %d, failed %d)", r.Imported, r.Skipped, r.Failures)
	}
	if r.Checkpoint != "" {
		fmt.Fprintf(&b, ".\ncheckpoint: %s", r.Checkpoint)
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

func draftHuman(r *DraftResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s draft for opportunity %s\n", r.Kind, r.OpportunityID)
	fmt.Fprintf(&b, "Title: %s\n", r.Title)
	fmt.Fprintf(&b, "--- Body ---\n%s\n--- End ---\n", r.Body)
	fmt.Fprintf(&b, "Rendered: %s", r.RenderedAt)
	return b.String()
}

func clusterListHuman(r *ClusterListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Clusters for %s: %d found", r.Repo, r.Total)
	for _, cl := range r.Clusters {
		fmt.Fprintf(&b, "\n- %s [%s] %s/%s:%s#%d (%d members)", cl.StableID, cl.State, cl.Canonical.Owner, cl.Canonical.Repo, cl.Canonical.Kind, cl.Canonical.Number, cl.MemberCount)
	}
	return b.String()
}

func clusterHuman(r *ClusterResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cluster: %s (%s)\n", r.StableID, r.State)
	fmt.Fprintf(&b, "Canonical: %s/%s:%s#%d\n", r.Canonical.Owner, r.Canonical.Repo, r.Canonical.Kind, r.Canonical.Number)
	fmt.Fprintf(&b, "Members: %d\n", r.MemberCount)
	for _, m := range r.Members {
		included := ""
		if !m.Included {
			included = " [excluded]"
		}
		fmt.Fprintf(&b, "- %s/%s:%s#%d: %s (%.2f) %s%s\n", m.Owner, m.Repo, m.Kind, m.Number, m.Title, m.Score, m.Reason, included)
	}
	return strings.TrimSpace(b.String())
}

func lensHuman(r *LensResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lens: %s\n", r.Name)
	f := r.Definition.Filter
	if len(f.Kinds) > 0 {
		fmt.Fprintf(&b, "Kinds: %s\n", strings.Join(f.Kinds, ", "))
	}
	if len(f.States) > 0 {
		fmt.Fprintf(&b, "States: %s\n", strings.Join(f.States, ", "))
	}
	if len(f.Languages) > 0 {
		fmt.Fprintf(&b, "Languages: %s\n", strings.Join(f.Languages, ", "))
	}
	if f.ExcludeArchived {
		fmt.Fprintln(&b, "Exclude archived: true")
	}
	if f.Unassigned {
		fmt.Fprintln(&b, "Unassigned: true")
	}
	if f.UpdatedWithin > 0 {
		fmt.Fprintf(&b, "Updated within: %s\n", f.UpdatedWithin)
	}
	if f.MinStars > 0 {
		fmt.Fprintf(&b, "Minimum stars: %d\n", f.MinStars)
	}
	if r.Definition.MaxResultsPerRepo > 0 {
		fmt.Fprintf(&b, "Max results per repo: %d\n", r.Definition.MaxResultsPerRepo)
	}
	if len(r.Definition.Weights) > 0 {
		var names []string
		for name := range r.Definition.Weights {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "Weights: %s", strings.Join(names, ", "))
	}
	return b.String()
}

func lensListHuman(r *LensListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d lenses", len(r.Lenses))
	for _, l := range r.Lenses {
		fmt.Fprintf(&b, "\n- %s", l.Name)
	}
	return b.String()
}

func lensExplainHuman(r *LensExplainResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lens: %s\n", r.Lens.Name)
	c := r.Candidate
	fmt.Fprintf(&b, "Candidate: %s %s", c.Kind, c.Repo)
	if c.Number > 0 {
		fmt.Fprintf(&b, "#%d", c.Number)
	}
	fmt.Fprintf(&b, ": %s\n", c.Title)
	if c.State != "" {
		fmt.Fprintf(&b, "State: %s\n", c.State)
	}
	if c.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", c.URL)
	}
	if c.UpdatedAt != "" {
		fmt.Fprintf(&b, "Updated: %s\n", c.UpdatedAt)
	}
	fmt.Fprintf(&b, "Population: %d (%s), evaluated at %s\n", r.PopulationSize, r.PopulationScope, r.EvaluatedAt)
	if r.Query != "" {
		fmt.Fprintf(&b, "Query: %s\n", r.Query)
	}
	fmt.Fprintf(&b, "Final score: %.2f\n", r.Score)
	if len(r.Signals) > 0 {
		b.WriteString("Signals:\n")
		for _, sig := range r.Signals {
			status := ""
			if sig.Missing {
				status = " [missing]"
			}
			fmt.Fprintf(&b, "- %s: value=%.3f normalized=%.3f weight=%.3f contribution=%.3f%s\n", sig.Name, sig.Value, sig.Normalized, sig.Weight, sig.Contribution, status)
		}
	}
	if len(r.MissingSignals) > 0 {
		fmt.Fprintf(&b, "Missing signals: %s", strings.Join(r.MissingSignals, ", "))
	}
	return strings.TrimSpace(b.String())
}

func collectionHuman(r *CollectionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Collection: %s\n", r.Name)
	fmt.Fprintf(&b, "Members: %d\n", r.MemberCount)
	fmt.Fprintf(&b, "Created: %s\nUpdated: %s", r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func collectionListHuman(r *CollectionListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d collections", len(r.Collections))
	for _, col := range r.Collections {
		fmt.Fprintf(&b, "\n- %s (%d members)", col.Name, col.MemberCount)
	}
	return b.String()
}

func triageEventHuman(r *TriageEventResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Triage event: %s (%s)\n", r.ID, r.Outcome)
	fmt.Fprintf(&b, "Target: %s %s\n", r.TargetKind, r.TargetRef)
	if r.Lens != "" {
		fmt.Fprintf(&b, "Lens: %s\n", r.Lens)
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "Reason: %s\n", r.Reason)
	}
	fmt.Fprintf(&b, "Created: %s\nUpdated: %s", r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func triageEventListHuman(r *TriageEventListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d triage events (limit %d)", len(r.Events), r.Limit)
	for _, e := range r.Events {
		fmt.Fprintf(&b, "\n- %s %s=%s [%s]", e.ID, e.TargetKind, e.TargetRef, e.Outcome)
		if e.Lens != "" {
			fmt.Fprintf(&b, " (lens: %s)", e.Lens)
		}
		if e.Reason != "" {
			fmt.Fprintf(&b, ": %s", e.Reason)
		}
	}
	return b.String()
}

func contributionHuman(r *ContributionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Contribution: %s (%s)\n", r.ID, r.Kind)
	fmt.Fprintf(&b, "Opportunity: %s\n", r.OpportunityID)
	fmt.Fprintf(&b, "Title: %s\n", r.Title)
	if r.Reference != "" {
		fmt.Fprintf(&b, "Reference: %s\n", r.Reference)
	}
	if r.ReferenceURL != "" {
		fmt.Fprintf(&b, "Reference URL: %s\n", r.ReferenceURL)
	}
	fmt.Fprintf(&b, "Prepared: %s\nCreated: %s\nUpdated: %s", r.PreparedAt, r.CreatedAt, r.UpdatedAt)
	return b.String()
}

func contributionListHuman(r *ContributionListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d contributions (limit %d)", len(r.Contributions), r.Limit)
	for _, c := range r.Contributions {
		fmt.Fprintf(&b, "\n- %s [%s] %s (opportunity %s)", c.ID, c.Kind, c.Title, c.OpportunityID)
	}
	return b.String()
}

func contributionOutcomeHuman(r *ContributionOutcomeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Contribution outcome: %s (%s)\n", r.ID, r.Outcome)
	fmt.Fprintf(&b, "Contribution: %s\n", r.ContributionID)
	if r.Reason != "" {
		fmt.Fprintf(&b, "Reason: %s\n", r.Reason)
	}
	fmt.Fprintf(&b, "Created: %s", r.CreatedAt)
	return b.String()
}

func contributionOutcomeListHuman(r *ContributionOutcomeListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d outcomes for contribution %s", len(r.Outcomes), r.ContributionID)
	for _, o := range r.Outcomes {
		fmt.Fprintf(&b, "\n- %s [%s]", o.Outcome, o.CreatedAt)
		if o.Reason != "" {
			fmt.Fprintf(&b, ": %s", o.Reason)
		}
	}
	return b.String()
}

func metadataImportHuman(r *MetadataImportResult) string {
	return fmt.Sprintf("Imported tracking bundle v%d: %d triage events, %d contributions, %d contribution outcomes, %d evidence records",
		r.SchemaVersion, r.TriageEvents, r.Contributions, r.ContributionOutcomes, r.Evidence)
}
