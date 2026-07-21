package cli

import (
	"fmt"
	"strings"
)

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
