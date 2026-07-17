package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
)

// ExportDossierJSON writes a deterministic, redacted JSON representation of d
// to w.
func ExportDossierJSON(w io.Writer, d *domain.Dossier) error {
	if d == nil {
		return errors.New("dossier is nil")
	}
	rd := redact(d).(*domain.Dossier)
	orderDossier(rd)
	b, err := json.MarshalIndent(rd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dossier: %w", err)
	}
	_, err = w.Write(b)
	return err
}

// ExportDossierMarkdown writes a deterministic, redacted Markdown report for d
// to w.
func ExportDossierMarkdown(w io.Writer, d *domain.Dossier) error {
	if d == nil {
		return errors.New("dossier is nil")
	}
	rd := redact(d).(*domain.Dossier)
	orderDossier(rd)
	return writeDossierMarkdown(w, rd)
}

// ExportEvidenceJSON writes a deterministic, redacted JSON representation of an
// evidence packet to w.
func ExportEvidenceJSON(w io.Writer, e *cli.EvidenceResult) error {
	if e == nil {
		return errors.New("evidence result is nil")
	}
	re := redact(e).(*cli.EvidenceResult)
	orderEvidence(re)
	b, err := json.MarshalIndent(re, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	_, err = w.Write(b)
	return err
}

// ExportEvidenceMarkdown writes a deterministic, redacted Markdown report for an
// evidence packet to w.
func ExportEvidenceMarkdown(w io.Writer, e *cli.EvidenceResult) error {
	if e == nil {
		return errors.New("evidence result is nil")
	}
	re := redact(e).(*cli.EvidenceResult)
	orderEvidence(re)
	return writeEvidenceMarkdown(w, re)
}

func orderDossier(d *domain.Dossier) {
	sort.SliceStable(d.SourceRefs, func(i, j int) bool {
		a, b := d.SourceRefs[i], d.SourceRefs[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.URL != b.URL {
			return a.URL < b.URL
		}
		if !a.ObservedAt.Equal(b.ObservedAt) {
			return a.ObservedAt.Before(b.ObservedAt)
		}
		if !a.AsOf.Equal(b.AsOf) {
			return a.AsOf.Before(b.AsOf)
		}
		return a.CommitSHA < b.CommitSHA
	})
	sort.SliceStable(d.Coverage.Facets, func(i, j int) bool {
		return d.Coverage.Facets[i].Facet < d.Coverage.Facets[j].Facet
	})
	sortThreads(d.RecentMergedPullRequests)
	sortThreads(d.RecentOpenPullRequests)
	sortThreads(d.RecentClosedUnmergedPullRequests)
	sortThreads(d.RecentIssues)
}

func sortThreads(t []domain.DossierThread) {
	sort.SliceStable(t, func(i, j int) bool {
		a, b := t[i], t[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.Number != b.Number {
			return a.Number > b.Number
		}
		return a.Title < b.Title
	})
}

func orderEvidence(e *cli.EvidenceResult) {
	sort.SliceStable(e.Evidence, func(i, j int) bool {
		return e.Evidence[i].ID < e.Evidence[j].ID
	})
}

func writeDossierMarkdown(w io.Writer, d *domain.Dossier) error {
	var b strings.Builder

	fmt.Fprintf(&b, "# Repository dossier: %s\n\n", d.Repo.String())
	fmt.Fprintf(&b, "- **Commit:** %s\n", d.CommitSHA)
	fmt.Fprintf(&b, "- **As of:** %s\n", formatTime(d.AsOf))

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Repository")
	fmt.Fprintf(&b, "- **Description:** %s\n", nonEmpty(d.Repository.Description))
	fmt.Fprintf(&b, "- **Languages:** %s\n", strings.Join(d.Repository.Languages, ", "))
	fmt.Fprintf(&b, "- **Default branch:** %s\n", d.Repository.DefaultBranch)
	fmt.Fprintf(&b, "- **License:** %s\n", nonEmpty(d.Repository.License))
	fmt.Fprintf(&b, "- **Stars:** %d, **Watchers:** %d, **Forks:** %d\n", d.Repository.Stars, d.Repository.Watchers, d.Repository.Forks)
	if d.Repository.Archived {
		fmt.Fprintln(&b, "- **Archived:** true")
	}
	if d.Repository.Fork {
		fmt.Fprintln(&b, "- **Fork:** true")
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Counts")
	fmt.Fprintf(&b, "- **Open issues:** %d\n", d.OpenIssueCount)
	fmt.Fprintf(&b, "- **Closed issues:** %d\n", d.ClosedIssueCount)
	fmt.Fprintf(&b, "- **Open pull requests:** %d\n", d.OpenPullRequestCount)
	fmt.Fprintf(&b, "- **Merged pull requests:** %d\n", d.MergedPullRequestCount)
	fmt.Fprintf(&b, "- **Closed unmerged pull requests:** %d\n", d.ClosedUnmergedPullRequestCount)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Source references")
	if len(d.SourceRefs) == 0 {
		fmt.Fprintln(&b, "_No source references recorded._")
	} else {
		for _, ref := range d.SourceRefs {
			fmt.Fprintf(&b, "- **%s** (%s) at %s", ref.Source, ref.URL, formatTime(ref.ObservedAt))
			if ref.CommitSHA != "" {
				fmt.Fprintf(&b, ", commit %s", ref.CommitSHA)
			}
			fmt.Fprintln(&b)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Coverage")
	fmt.Fprintf(&b, "- **As of:** %s\n", formatTime(d.Coverage.AsOf))
	if len(d.Coverage.Facets) == 0 {
		fmt.Fprintln(&b, "_No coverage recorded._")
	} else {
		for _, f := range d.Coverage.Facets {
			fmt.Fprintf(&b, "- **%s:** present=%v, complete=%v, freshness=%s, as_of=%s", f.Facet, f.Present, f.Complete, f.Freshness.Status, formatTime(f.Freshness.AsOf))
			if f.Count > 0 {
				fmt.Fprintf(&b, ", count=%d", f.Count)
			}
			fmt.Fprintln(&b)
		}
	}

	if d.ContributionGuidance != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Contribution guidance")
		fmt.Fprintln(&b, d.ContributionGuidance)
	}

	writeThreadSection(&b, "Recent merged pull requests", d.RecentMergedPullRequests)
	writeThreadSection(&b, "Recent open pull requests", d.RecentOpenPullRequests)
	writeThreadSection(&b, "Recent closed unmerged pull requests", d.RecentClosedUnmergedPullRequests)
	writeThreadSection(&b, "Recent issues", d.RecentIssues)

	_, err := w.Write([]byte(b.String()))
	return err
}

func writeThreadSection(b *strings.Builder, heading string, threads []domain.DossierThread) {
	fmt.Fprintln(b)
	fmt.Fprintf(b, "## %s\n", heading)
	if len(threads) == 0 {
		fmt.Fprintln(b, "_No entries._")
		return
	}
	for _, t := range threads {
		fmt.Fprintf(b, "- **#%d** %s (%s", t.Number, t.Title, t.State)
		if t.Draft {
			fmt.Fprint(b, ", draft")
		}
		if t.Author != "" {
			fmt.Fprintf(b, ", by %s", t.Author)
		}
		if len(t.Labels) > 0 {
			fmt.Fprintf(b, ", labels: %s", strings.Join(t.Labels, ", "))
		}
		fmt.Fprintf(b, ") — updated %s", formatTime(t.UpdatedAt))
		if !t.ClosedAt.IsZero() {
			fmt.Fprintf(b, ", closed %s", formatTime(t.ClosedAt))
		}
		if !t.MergedAt.IsZero() {
			fmt.Fprintf(b, ", merged %s", formatTime(t.MergedAt))
		}
		fmt.Fprintln(b)
	}
}

func writeEvidenceMarkdown(w io.Writer, e *cli.EvidenceResult) error {
	var b strings.Builder

	fmt.Fprintf(&b, "# Investigation evidence: %s\n\n", e.InvestigationID)
	if len(e.Evidence) == 0 {
		fmt.Fprintln(&b, "_No evidence recorded._")
		_, err := w.Write([]byte(b.String()))
		return err
	}

	for _, item := range e.Evidence {
		fmt.Fprintf(&b, "- **%s** (`%s`, relation `%s`)", item.ID, item.Type, item.Relation)
		if item.Description != "" {
			fmt.Fprintf(&b, ": %s", item.Description)
		}
		if item.ValidationRunID != "" {
			fmt.Fprintf(&b, " [run: %s]", item.ValidationRunID)
		}
		if item.OpportunityID != "" {
			fmt.Fprintf(&b, " [opportunity: %s]", item.OpportunityID)
		}
		if item.CreatedAt != "" {
			fmt.Fprintf(&b, " — %s", item.CreatedAt)
		}
		fmt.Fprintln(&b)
	}

	_, err := w.Write([]byte(b.String()))
	return err
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func nonEmpty(s string) string {
	if s == "" {
		return "_not provided_"
	}
	return s
}
