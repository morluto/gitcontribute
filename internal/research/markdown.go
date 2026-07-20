package research

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/morluto/gitcontribute/internal/redaction"
)

// RenderMarkdown writes a deterministic, redacted research brief. Untrusted
// source excerpts are quoted and HTML-escaped so they remain data.
func RenderMarkdown(w io.Writer, brief *Brief) error {
	if w == nil {
		return errors.New("markdown writer is required")
	}
	if brief == nil {
		return errors.New("research brief is required")
	}
	if err := brief.ValidateProvenance(); err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Thread research brief: %s\n\n", inline(brief.Target.Ref))
	fmt.Fprintf(&b, "- **Schema:** %s\n", inline(brief.SchemaVersion))
	fmt.Fprintf(&b, "- **Generated:** %s\n", formatMarkdownTime(brief.GeneratedAt))
	fmt.Fprintf(&b, "- **Source as of:** %s\n", formatMarkdownTime(brief.SourceAsOf))
	fmt.Fprintf(&b, "- **Thread:** [%s](%s)\n", inline(brief.Target.Ref), safeURL(brief.Target.URL))

	writeCurrentState(&b, brief.Sections.CurrentState)
	writeProblem(&b, brief.Sections.Problem)
	writeAcceptance(&b, brief.Sections.Acceptance)
	writeParticipants(&b, brief.Sections.Participants)
	writeTimeline(&b, brief.Sections.Timeline)
	writeDuplicates(&b, brief.Sections.Duplicates)
	writePullRequests(&b, brief.Sections.PullRequests)
	writeCode(&b, brief.Sections.Code)
	writeGuidance(&b, brief.Sections.Guidance)
	writeHealth(&b, brief.Sections.Health)
	writeCoverage(&b, brief.Sections.Coverage)
	writeNext(&b, brief.Sections.Next)

	_, err := io.WriteString(w, b.String())
	return err
}

func writeCurrentState(b *strings.Builder, section CurrentStateSection) {
	sectionHeading(b, "1. Current state", section.SectionMeta)
	fmt.Fprintf(b, "- **State:** %s\n", inline(section.State))
	if section.StateReason != "" {
		fmt.Fprintf(b, "- **State reason:** %s\n", inline(section.StateReason))
	}
	merged := "unknown"
	if section.Merged != nil {
		merged = strconv.FormatBool(*section.Merged)
	}
	fmt.Fprintf(b, "- **Draft / locked / merged:** %t / %t / %s\n", section.Draft, section.Locked, merged)
	writeStringList(b, "Labels", section.Labels)
	if section.Milestone != "" {
		fmt.Fprintf(b, "- **Milestone:** %s\n", inline(section.Milestone))
	}
	writeOptionalTime(b, "Created", section.CreatedAt)
	writeOptionalTime(b, "Updated", section.UpdatedAt)
	writeOptionalTime(b, "Closed", section.ClosedAt)
	writeOptionalTime(b, "Merged", section.MergedAt)
}

func writeProblem(b *strings.Builder, section ProblemSection) {
	sectionHeading(b, "2. Problem statement (stored extract)", section.SectionMeta)
	fmt.Fprintf(b, "- **Title:** %s\n", inline(section.Title))
	writeStringList(b, "Labels", section.Labels)
	writeStringList(b, "Assignees", section.Assignees)
	if section.BodyExcerpt == "" {
		fmt.Fprintln(b, "\n_No stored body._")
		return
	}
	fmt.Fprint(b, "\n**Bounded body excerpt:**\n\n")
	writeQuote(b, section.BodyExcerpt)
}

func writeAcceptance(b *strings.Builder, section AcceptanceSection) {
	sectionHeading(b, "3. Acceptance hints", section.SectionMeta)
	fmt.Fprintf(b, "_%s_\n", inline(section.Caveat))
	fmt.Fprintln(b, "\n**Checkboxes:**")
	if len(section.Checklist) == 0 {
		fmt.Fprintln(b, "\n- None extracted.")
	} else {
		for _, item := range section.Checklist {
			mark := " "
			if item.Checked {
				mark = "x"
			}
			fmt.Fprintf(b, "\n- [%s] %s\n", mark, inline(item.Text))
		}
	}
	fmt.Fprintln(b, "\n**Relevant source headings:**")
	writeTextHints(b, section.RelevantHeadings)
	fmt.Fprintln(b, "\n**Maintainer statements matching acceptance language:**")
	writeTextHints(b, section.MaintainerStatements)
}

func writeParticipants(b *strings.Builder, section ParticipantsSection) {
	sectionHeading(b, "4. Participants", section.SectionMeta)
	if len(section.Participants) == 0 {
		fmt.Fprintln(b, "_No participant identities stored._")
		return
	}
	for _, participant := range section.Participants {
		association := "association unknown"
		if participant.Association != "" {
			association = participant.Association
		}
		fmt.Fprintf(b, "- **%s** — %s; roles: %s\n", inline(participant.Login), inline(association), inline(strings.Join(participant.Roles, ", ")))
	}
}

func writeTimeline(b *strings.Builder, section TimelineSection) {
	sectionHeading(b, "5. Timeline", section.SectionMeta)
	if len(section.Events) == 0 {
		fmt.Fprintln(b, "_No dated events stored._")
		return
	}
	for _, event := range section.Events {
		actor := ""
		if event.Actor != "" {
			actor = " by " + inline(event.Actor)
		}
		fmt.Fprintf(b, "- **%s** — %s%s: %s\n", formatMarkdownTime(event.At), inline(event.Kind), actor, inline(event.Summary))
	}
}

func writeDuplicates(b *strings.Builder, section DuplicateSection) {
	sectionHeading(b, "6. Duplicate candidates and explicit references", section.SectionMeta)
	fmt.Fprintf(b, "_%s_\n", inline(section.Caveat))
	if section.ClusterID != "" {
		fmt.Fprintf(b, "\n- **Stored cluster:** %s\n", inline(section.ClusterID))
	}
	if section.Canonical != "" {
		fmt.Fprintf(b, "- **Canonical candidate:** %s\n", inline(section.Canonical))
	}
	writeRelatedThreads(b, section.Candidates)
}

func writePullRequests(b *strings.Builder, section PullRequestSection) {
	sectionHeading(b, "7. Competing or linked pull requests", section.SectionMeta)
	writeRelatedThreads(b, section.PullRequests)
}

func writeCode(b *strings.Builder, section CodeSection) {
	sectionHeading(b, "8. Relevant indexed code", section.SectionMeta)
	if section.CommitSHA != "" {
		fmt.Fprintf(b, "- **Indexed commit:** `%s`\n", code(section.CommitSHA))
	}
	writeStringList(b, "Queries", section.Queries)
	if len(section.Hits) == 0 {
		fmt.Fprintln(b, "\n_No relevant code hits._")
		return
	}
	for _, hit := range section.Hits {
		language := ""
		if hit.Language != "" {
			language = " (" + inline(hit.Language) + ")"
		}
		fmt.Fprintf(b, "- `%s`%s — term `%s`, commit `%s`\n", code(hit.Path), language, code(hit.MatchedTerm), code(hit.CommitSHA))
	}
}

func writeGuidance(b *strings.Builder, section GuidanceSection) {
	sectionHeading(b, "9. Contribution and AI guidance", section.SectionMeta)
	if section.Text == "" {
		fmt.Fprintln(b, "_No stored guidance; none was inferred._")
		return
	}
	writeQuote(b, section.Text)
}

func writeHealth(b *strings.Builder, section HealthSection) {
	sectionHeading(b, "10. Repository health and responsiveness", section.SectionMeta)
	fmt.Fprintf(b, "- **Archived:** %t\n", section.Archived)
	fmt.Fprintf(b, "- **Open issues / PRs:** %d / %d\n", section.OpenIssues, section.OpenPullRequests)
	if section.ExternalPRMergeRate == nil {
		fmt.Fprintf(b, "- **External PR merge rate:** unknown (sample=%d)\n", section.ExternalPRSampleSize)
	} else {
		fmt.Fprintf(b, "- **External PR merge rate:** %.2f (sample=%d)\n", *section.ExternalPRMergeRate, section.ExternalPRSampleSize)
	}
	fmt.Fprintf(b, "- **Median first response, issue / PR:** %.2fh / %.2fh (samples=%d / %d)\n", section.IssueResponseMedianHours, section.PullRequestResponseMedianHours, section.IssueResponseSampleSize, section.PullRequestResponseSampleSize)
	fmt.Fprintf(b, "- **Thread sample:** %d (truncated=%t)\n", section.ThreadSampleSize, section.ThreadsTruncated)
}

func writeCoverage(b *strings.Builder, section CoverageSection) {
	sectionHeading(b, "11. Coverage and gaps", section.SectionMeta)
	if len(section.Facets) == 0 {
		fmt.Fprintln(b, "_No coverage facts stored._")
	} else {
		for _, facet := range section.Facets {
			fmt.Fprintf(b, "- **%s:%s:** present=%t, complete=%t, truncated=%t, count=%d", inline(facet.Scope), inline(facet.Facet), facet.Present, facet.Complete, facet.Truncated, facet.Count)
			if !facet.AsOf.IsZero() {
				fmt.Fprintf(b, ", as_of=%s", formatMarkdownTime(facet.AsOf))
			}
			fmt.Fprintln(b)
		}
	}
	writeStringList(b, "Gaps", section.Gaps)
}

func writeNext(b *strings.Builder, section NextSection) {
	sectionHeading(b, "12. Next explicit commands", section.SectionMeta)
	if len(section.Commands) == 0 {
		fmt.Fprintln(b, "_No follow-on command generated._")
		return
	}
	for _, command := range section.Commands {
		fmt.Fprintf(b, "- %s\n\n  ```console\n  %s\n  ```\n", inline(command.Reason), code(command.Command))
	}
}

func sectionHeading(b *strings.Builder, title string, meta SectionMeta) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	fmt.Fprintf(b, "- **Evidence status:** %s\n", inline(string(meta.Status)))
	if meta.UnknownReason != "" {
		fmt.Fprintf(b, "- **Coverage note:** %s\n", inline(meta.UnknownReason))
	}
	fmt.Fprintln(b, "- **Sources:**")
	if len(meta.Sources) == 0 {
		fmt.Fprintln(b, "  - Unknown (reason recorded above)")
		return
	}
	for _, source := range meta.Sources {
		label := inline(source.Source)
		if label == "" {
			label = "source"
		}
		fmt.Fprintf(b, "  - %s", label)
		if source.URL != "" {
			fmt.Fprintf(b, ": <%s>", safeURL(source.URL))
		}
		if source.CommitSHA != "" {
			fmt.Fprintf(b, ", commit `%s`", code(source.CommitSHA))
		}
		if !source.AsOf.IsZero() {
			fmt.Fprintf(b, ", as of %s", formatMarkdownTime(source.AsOf))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func writeTextHints(b *strings.Builder, hints []TextHint) {
	if len(hints) == 0 {
		fmt.Fprintln(b, "\n- None extracted.")
		return
	}
	for _, hint := range hints {
		author := ""
		if hint.Author != "" {
			author = " — " + inline(hint.Author)
		}
		fmt.Fprintf(b, "\n- %s%s\n", inline(hint.Text), author)
	}
}

func writeRelatedThreads(b *strings.Builder, threads []RelatedThread) {
	if len(threads) == 0 {
		fmt.Fprintln(b, "\n_No related threads found in the bounded local evidence._")
		return
	}
	for _, thread := range threads {
		title := inline(thread.Title)
		if title == "" {
			title = inline(thread.Ref)
		}
		fmt.Fprintf(b, "\n- [%s](%s) — %s; %s", title, safeURL(thread.URL), inline(thread.Relation), inline(thread.Basis))
		if thread.State != "" {
			fmt.Fprintf(b, "; state=%s", inline(thread.State))
		}
		fmt.Fprintln(b)
	}
}

func writeQuote(b *strings.Builder, text string) {
	text = safeBlock(text)
	if text == "" {
		fmt.Fprintln(b, "> _empty_")
		return
	}
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "> %s\n", line)
	}
}

func writeStringList(b *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "- **%s:** none\n", inline(label))
		return
	}
	escaped := make([]string, len(values))
	for i, value := range values {
		escaped[i] = inline(value)
	}
	fmt.Fprintf(b, "- **%s:** %s\n", inline(label), strings.Join(escaped, ", "))
}

func writeOptionalTime(b *strings.Builder, label string, value time.Time) {
	if !value.IsZero() {
		fmt.Fprintf(b, "- **%s:** %s\n", inline(label), formatMarkdownTime(value))
	}
}

func inline(value string) string {
	value = redaction.String(cleanControls(value))
	value = strings.Join(strings.Fields(value), " ")
	replacer := strings.NewReplacer(
		`\`, `\\`, "`", `\`+"`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", "&lt;", ">", "&gt;", "#", `\#`,
	)
	return replacer.Replace(value)
}

func safeBlock(value string) string {
	value = redaction.String(cleanControls(value))
	return html.EscapeString(value)
}

func safeURL(value string) string {
	value = redaction.String(cleanControls(value))
	value = strings.Join(strings.Fields(value), "")
	value = strings.ReplaceAll(value, "<", "%3C")
	value = strings.ReplaceAll(value, ">", "%3E")
	return value
}

func code(value string) string {
	value = redaction.String(cleanControls(value))
	value = strings.Join(strings.Fields(value), " ")
	value = strings.ReplaceAll(value, "`", "'")
	return value
}

func cleanControls(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, value)
}

func formatMarkdownTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339)
}
