package research

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

const (
	maxAcceptanceHints = 50
	maxTimelineEvents  = 100
	maxCodeTerms       = 5
	maxExplicitRefs    = 50
)

var (
	checklistPattern = regexp.MustCompile(`(?m)^\s*[-*]\s*\[([ xX])\]\s*(.+?)\s*$`)
	headingPattern   = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	hintPattern      = regexp.MustCompile(`(?i)\b(acceptance|criteria|expected|actual|required|requirements?|definition of done|must|should|please|need(?:s|ed)?)\b`)
	codeTokenPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_.-]{2,}`)
)

var codeStopWords = map[string]struct{}{
	"about": {}, "after": {}, "before": {}, "could": {}, "error": {},
	"feature": {}, "from": {}, "have": {}, "into": {}, "issue": {},
	"should": {}, "that": {}, "the": {}, "this": {}, "with": {},
}

// Builder assembles a research brief from already stored, source-backed facts.
type Builder struct {
	reader Reader
	clock  func() time.Time
}

// NewBuilder returns a deterministic research brief builder.
func NewBuilder(reader Reader, clock func() time.Time) *Builder {
	if clock == nil {
		clock = time.Now
	}
	return &Builder{reader: reader, clock: clock}
}

// Build reads only the capabilities exposed by Reader and produces a fixed v1
// brief. Source content is treated as data, never as instructions.
func (b *Builder) Build(ctx context.Context, requested ThreadRef) (*Brief, error) {
	if b == nil || b.reader == nil {
		return nil, errors.New("research reader is required")
	}
	if err := requested.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repo, repoSources, err := b.reader.ReadRepository(ctx, requested.Repo)
	if err != nil {
		return nil, fmt.Errorf("read repository: %w", err)
	}
	repoCoverage, err := b.reader.ReadCoverage(ctx, requested.Repo)
	if err != nil {
		return nil, fmt.Errorf("read repository coverage: %w", err)
	}
	guidance, guidanceSources, err := b.reader.ReadContributionGuidance(ctx, requested.Repo)
	if err != nil {
		return nil, fmt.Errorf("read contribution guidance: %w", err)
	}
	thread, err := b.reader.ReadResearchThread(ctx, requested)
	if err != nil {
		return nil, fmt.Errorf("read thread: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	explicit := extractReferences(thread)
	relations, err := b.reader.ReadResearchRelationships(ctx, thread.Thread.Ref, explicit)
	if err != nil {
		return nil, fmt.Errorf("read thread relationships: %w", err)
	}
	terms := codeTerms(thread.Thread.Title, thread.Thread.Labels)
	code, err := b.reader.ReadResearchCode(ctx, requested.Repo, terms)
	if err != nil {
		return nil, fmt.Errorf("read relevant code: %w", err)
	}
	health, err := b.reader.ReadResearchHealth(ctx, requested.Repo)
	if err != nil {
		return nil, fmt.Errorf("read repository health: %w", err)
	}

	brief := assembleBrief(assemblyInput{
		now:             b.clock().UTC(),
		repo:            repo,
		repoSources:     fromDomainSources(repoSources),
		repoCoverage:    repoCoverage,
		guidance:        guidance,
		guidanceSources: fromDomainSources(guidanceSources),
		thread:          thread,
		relations:       relations,
		code:            code,
		health:          health,
	})
	if err := brief.ValidateProvenance(); err != nil {
		return nil, fmt.Errorf("validate research brief: %w", err)
	}
	return brief, nil
}

type assemblyInput struct {
	now             time.Time
	repo            domain.Repository
	repoSources     []SourceRef
	repoCoverage    domain.Coverage
	guidance        string
	guidanceSources []SourceRef
	thread          ThreadEvidence
	relations       RelationshipEvidence
	code            CodeEvidence
	health          HealthEvidence
}

func assembleBrief(in assemblyInput) *Brief {
	t := in.thread.Thread
	discussionGap := discussionCoverageGap(t.Ref.Kind, in.thread.Coverage)
	if in.thread.Truncated {
		discussionGap = joinReasons(discussionGap, "discussion evidence was bounded for this brief")
	}

	brief := &Brief{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   in.now,
		Target: Target{
			Ref: t.Ref.String(), Repository: t.Ref.Repo.String(), Kind: string(t.Ref.Kind),
			Number: t.Ref.Number, URL: threadURL(t.Ref),
		},
	}
	brief.Sections.CurrentState = buildCurrentState(t)
	brief.Sections.Problem = buildProblem(t)
	brief.Sections.Acceptance = buildAcceptance(in.thread, discussionGap)
	brief.Sections.Participants = buildParticipants(in.thread, discussionGap)
	brief.Sections.Timeline = buildTimeline(in.thread, discussionGap)
	brief.Sections.Duplicates = buildDuplicates(in.relations)
	brief.Sections.PullRequests = buildPullRequests(in.relations)
	brief.Sections.Code = buildCode(in.code)
	brief.Sections.Guidance = buildGuidance(in.guidance, in.guidanceSources)
	brief.Sections.Health = buildHealth(in.health)
	brief.Sections.Coverage = buildCoverage(in, discussionGap)
	brief.Sections.Next = buildNext(in, discussionGap)

	allSources := discussionSources(in.thread)
	allSources = append(allSources, in.repoSources...)
	allSources = append(allSources, in.guidanceSources...)
	allSources = append(allSources, in.relations.Sources...)
	allSources = append(allSources, in.health.Sources...)
	if in.code.Present {
		allSources = append(allSources, in.code.Source)
	}
	brief.SourceAsOf = latestSourceTime(allSources)
	return brief
}

func buildCurrentState(t ThreadSnapshot) CurrentStateSection {
	return CurrentStateSection{
		SectionMeta: sourceMeta([]SourceRef{t.Source}, ""),
		State:       t.State, StateReason: t.StateReason, Draft: t.Draft, Locked: t.Locked,
		Merged: t.Merged, Labels: cleanSorted(t.Labels), Milestone: t.Milestone,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, ClosedAt: t.ClosedAt, MergedAt: t.MergedAt,
	}
}

func buildProblem(t ThreadSnapshot) ProblemSection {
	return ProblemSection{
		SectionMeta: sourceMeta([]SourceRef{t.Source}, ""),
		Title:       t.Title, BodyExcerpt: boundedText(t.Body, MaximumBodyExcerpt),
		Labels: cleanSorted(t.Labels), Assignees: cleanSorted(t.Assignees),
	}
}

func buildAcceptance(e ThreadEvidence, gap string) AcceptanceSection {
	t := e.Thread
	section := AcceptanceSection{
		SectionMeta:          sourceMeta([]SourceRef{t.Source}, gap),
		Checklist:            []ChecklistHint{},
		RelevantHeadings:     []TextHint{},
		MaintainerStatements: []TextHint{},
		Caveat:               "Extracted source hints are not a complete acceptance contract.",
	}
	for _, match := range checklistPattern.FindAllStringSubmatch(t.Body, maxAcceptanceHints) {
		section.Checklist = append(section.Checklist, ChecklistHint{
			Text: boundedText(match[2], 500), Checked: strings.TrimSpace(match[1]) != "", Source: t.Source,
		})
	}
	for _, match := range headingPattern.FindAllStringSubmatch(t.Body, maxAcceptanceHints) {
		if hintPattern.MatchString(match[1]) {
			section.RelevantHeadings = append(section.RelevantHeadings, TextHint{Text: boundedText(match[1], 300), Source: t.Source})
		}
	}
	for _, item := range e.Discussion {
		if !maintainerAssociation(item.AuthorAssociation) || !hintPattern.MatchString(item.Body) {
			continue
		}
		section.MaintainerStatements = append(section.MaintainerStatements, TextHint{
			Text: boundedText(item.Body, 500), Author: item.Author, Source: item.Source,
		})
		section.Sources = append(section.Sources, item.Source)
		if len(section.MaintainerStatements) == maxAcceptanceHints {
			break
		}
	}
	section.Sources = normalizeSources(section.Sources)
	return section
}

func buildParticipants(e ThreadEvidence, gap string) ParticipantsSection {
	type entry struct {
		login       string
		association string
		roles       map[string]struct{}
	}
	participants := map[string]*entry{}
	add := func(login, association, role string) {
		login = strings.TrimSpace(login)
		if login == "" {
			return
		}
		key := strings.ToLower(login)
		if participants[key] == nil {
			participants[key] = &entry{login: login, roles: map[string]struct{}{}}
		}
		if participants[key].association == "" && association != "" {
			participants[key].association = association
		}
		participants[key].roles[role] = struct{}{}
	}
	add(e.Thread.Author, e.Thread.AuthorAssociation, "author")
	for _, assignee := range e.Thread.Assignees {
		add(assignee, "", "assignee")
	}
	for _, item := range e.Discussion {
		role := item.Kind
		switch role {
		case "issue_comment":
			role = "commenter"
		case "review":
			role = "reviewer"
		case "review_comment":
			role = "review_commenter"
		}
		add(item.Author, item.AuthorAssociation, role)
	}
	keys := make([]string, 0, len(participants))
	for key := range participants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Participant, 0, len(keys))
	for _, key := range keys {
		roles := make([]string, 0, len(participants[key].roles))
		for role := range participants[key].roles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		out = append(out, Participant{Login: participants[key].login, Association: participants[key].association, Roles: roles})
	}
	return ParticipantsSection{SectionMeta: sourceMeta(discussionSources(e), gap), Participants: out}
}

func buildTimeline(e ThreadEvidence, gap string) TimelineSection {
	t := e.Thread
	events := []TimelineEvent{}
	add := func(at time.Time, kind, actor, summary string, source SourceRef) {
		if !at.IsZero() {
			events = append(events, TimelineEvent{At: at.UTC(), Kind: kind, Actor: actor, Summary: summary, Source: source})
		}
	}
	add(t.CreatedAt, "created", t.Author, "thread created", t.Source)
	add(t.UpdatedAt, "updated", "", "thread source updated", t.Source)
	add(t.ClosedAt, "closed", "", "thread closed", t.Source)
	add(t.MergedAt, "merged", "", "pull request merged", t.Source)
	discussion := append([]DiscussionItem{}, e.Discussion...)
	sort.SliceStable(discussion, func(i, j int) bool {
		left, right := eventTime(discussion[i]), eventTime(discussion[j])
		if !left.Equal(right) {
			return left.Before(right)
		}
		if discussion[i].Kind != discussion[j].Kind {
			return discussion[i].Kind < discussion[j].Kind
		}
		return discussion[i].ID < discussion[j].ID
	})
	room := maxTimelineEvents - len(events)
	truncated := e.Truncated || len(discussion) > room
	if room < 0 {
		room = 0
	}
	if len(discussion) > room {
		discussion = discussion[len(discussion)-room:]
	}
	for _, item := range discussion {
		summary := strings.ReplaceAll(item.Kind, "_", " ")
		if item.State != "" {
			summary += " (" + item.State + ")"
		}
		add(eventTime(item), item.Kind, item.Author, summary, item.Source)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.Before(events[j].At)
		}
		if events[i].Kind != events[j].Kind {
			return events[i].Kind < events[j].Kind
		}
		return events[i].Source.URL < events[j].Source.URL
	})
	unknown := gap
	if truncated {
		unknown = joinReasons(unknown, "timeline was bounded to the most recent stored discussion events")
	}
	return TimelineSection{SectionMeta: sourceMeta(discussionSources(e), unknown), Events: events, Truncated: truncated}
}

func buildDuplicates(e RelationshipEvidence) DuplicateSection {
	unknown := ""
	if e.DuplicateCapped {
		unknown = "duplicate relationship scan reached its bound"
	}
	return DuplicateSection{
		SectionMeta: sourceMeta(e.Sources, unknown), ClusterID: e.ClusterID, Canonical: e.Canonical,
		Candidates: append([]RelatedThread{}, e.DuplicateThreads...), Truncated: e.DuplicateCapped,
		Caveat: "Explicit references and similarity clusters are candidates, not confirmed duplicate decisions.",
	}
}

func buildPullRequests(e RelationshipEvidence) PullRequestSection {
	unknown := ""
	if e.PullRequestCapped {
		unknown = "open pull-request relationship scan reached its bound"
	}
	return PullRequestSection{
		SectionMeta: sourceMeta(e.Sources, unknown), PullRequests: append([]RelatedThread{}, e.PullRequests...),
		Truncated: e.PullRequestCapped,
	}
}

func buildCode(e CodeEvidence) CodeSection {
	if !e.Present {
		return CodeSection{
			SectionMeta: sourceMeta(nil, "repository has no local code snapshot"),
			Queries:     cleanSorted(e.Queries), Hits: []CodeHit{},
		}
	}
	unknown := ""
	if e.Truncated {
		unknown = "code matches reached the per-brief bound"
	}
	return CodeSection{
		SectionMeta: sourceMeta([]SourceRef{e.Source}, unknown), CommitSHA: e.CommitSHA,
		Queries: cleanSorted(e.Queries), Hits: append([]CodeHit{}, e.Hits...), Truncated: e.Truncated,
	}
}

func buildGuidance(text string, sources []SourceRef) GuidanceSection {
	if strings.TrimSpace(text) == "" || len(sources) == 0 {
		return GuidanceSection{SectionMeta: sourceMeta(nil, "contribution and AI guidance is not present in the local corpus")}
	}
	return GuidanceSection{SectionMeta: sourceMeta(sources, ""), Text: boundedText(text, 5000)}
}

func buildHealth(e HealthEvidence) HealthSection {
	section := HealthSection{
		Archived: e.Archived, OpenIssues: e.OpenIssues, OpenPullRequests: e.OpenPullRequests,
		ExternalPRMergeRate: e.ExternalPRMergeRate, ExternalPRSampleSize: e.ExternalPRSampleSize,
		IssueResponseMedianHours:       e.IssueResponseMedianHours,
		PullRequestResponseMedianHours: e.PullRequestResponseMedianHours,
		IssueResponseSampleSize:        e.IssueResponseSampleSize,
		PullRequestResponseSampleSize:  e.PullRequestResponseSampleSize,
		ThreadSampleSize:               e.ThreadSampleSize, ThreadsTruncated: e.ThreadsTruncated,
	}
	if !e.Available {
		section.SectionMeta = sourceMeta(e.Sources, nonEmpty(e.UnknownReason, "repository health is unavailable"))
		return section
	}
	unknown := e.UnknownReason
	if e.ThreadsTruncated {
		unknown = joinReasons(unknown, "health metrics use a bounded thread population")
	}
	section.SectionMeta = sourceMeta(e.Sources, unknown)
	return section
}

func buildCoverage(in assemblyInput, discussionGap string) CoverageSection {
	facts := []CoverageFact{}
	gaps := []string{}
	repoFacets := map[string]domain.FacetCoverage{}
	for _, facet := range in.repoCoverage.Facets {
		repoFacets[facet.Facet] = facet
		facts = append(facts, CoverageFact{
			Scope: "repository", Facet: facet.Facet, Present: facet.Present, Complete: facet.Complete,
			AsOf: facet.Freshness.AsOf, Count: facet.Count,
		})
		if !facet.Present || !facet.Complete {
			gaps = append(gaps, "repository:"+facet.Facet)
		}
	}
	for _, required := range []string{"metadata", "threads"} {
		if _, ok := repoFacets[required]; !ok {
			facts = append(facts, CoverageFact{Scope: "repository", Facet: required})
			gaps = append(gaps, "repository:"+required)
		}
	}
	for _, facet := range in.thread.Coverage {
		facts = append(facts, CoverageFact{
			Scope: "thread", Facet: facet.Facet, Present: facet.Present, Complete: facet.Complete,
			Truncated: facet.Truncated, AsOf: facet.AsOf, Count: facet.Count,
		})
		if !facet.Present || !facet.Complete || facet.Truncated {
			gaps = append(gaps, "thread:"+facet.Facet)
		}
	}
	facts = append(facts, CoverageFact{
		Scope: "repository", Facet: "code_index", Present: in.code.Present, Complete: in.code.Present,
		AsOf: in.code.Source.AsOf, Count: len(in.code.Hits),
	})
	if !in.code.Present {
		gaps = append(gaps, "repository:code_index")
	}
	guidancePresent := strings.TrimSpace(in.guidance) != "" && len(in.guidanceSources) > 0
	facts = append(facts, CoverageFact{Scope: "repository", Facet: "contribution_guidance", Present: guidancePresent, Complete: guidancePresent})
	if !guidancePresent {
		gaps = append(gaps, "repository:contribution_guidance")
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Scope != facts[j].Scope {
			return facts[i].Scope < facts[j].Scope
		}
		return facts[i].Facet < facts[j].Facet
	})
	gaps = cleanSorted(gaps)
	sources := append([]SourceRef{}, in.repoSources...)
	sources = append(sources, discussionSources(in.thread)...)
	if in.code.Present {
		sources = append(sources, in.code.Source)
	}
	sources = append(sources, in.guidanceSources...)
	unknown := discussionGap
	if len(gaps) > 0 {
		unknown = joinReasons(unknown, "one or more research inputs are missing or incomplete")
	}
	return CoverageSection{SectionMeta: sourceMeta(sources, unknown), Facets: facts, Gaps: gaps}
}

func buildNext(in assemblyInput, discussionGap string) NextSection {
	ref := in.thread.Thread.Ref
	commands := []NextCommand{}
	missingFacets := []string{}
	for _, facet := range in.thread.Coverage {
		if !facet.Present || !facet.Complete {
			missingFacets = append(missingFacets, facet.Facet)
		}
	}
	if len(missingFacets) > 0 {
		commands = append(commands, NextCommand{
			Reason:  "hydrate missing or partial thread facets",
			Command: fmt.Sprintf("gitcontribute archive hydrate %s#%d --with %s", ref.Repo, ref.Number, strings.Join(cleanSorted(missingFacets), ",")),
		})
	}
	for _, required := range []string{"metadata", "threads"} {
		covered := false
		for _, facet := range in.repoCoverage.Facets {
			if facet.Facet == required && facet.Present && facet.Complete {
				covered = true
				break
			}
		}
		if !covered {
			commands = append(commands, NextCommand{
				Reason:  "refresh missing repository " + required + " coverage",
				Command: "gitcontribute archive sync " + ref.Repo.String() + " --state all",
			})
			break
		}
	}
	if !in.code.Present {
		commands = append(commands, NextCommand{Reason: "index a clean local checkout for code hits", Command: "gitcontribute index " + ref.Repo.String() + " ."})
	}
	if in.relations.PullRequestCapped || in.relations.DuplicateCapped {
		commands = append(commands, NextCommand{
			Reason:  "inspect additional local similarity candidates",
			Command: fmt.Sprintf("gitcontribute neighbors %s#%d --kind %s --limit 20", ref.Repo, ref.Number, ref.Kind),
		})
	}
	if len(commands) == 0 {
		commands = append(commands, NextCommand{
			Reason:  "inspect local similarity candidates before selecting work",
			Command: fmt.Sprintf("gitcontribute neighbors %s#%d --kind %s --limit 20", ref.Repo, ref.Number, ref.Kind),
		})
	}
	sources := append(discussionSources(in.thread), in.relations.Sources...)
	return NextSection{SectionMeta: sourceMeta(sources, discussionGap), Commands: commands}
}

func extractReferences(e ThreadEvidence) []Reference {
	type textSource struct {
		text   string
		source SourceRef
	}
	inputs := make([]textSource, 0, 1+len(e.Discussion))
	inputs = append(inputs, textSource{text: e.Thread.Title + "\n" + e.Thread.Body, source: e.Thread.Source})
	for _, item := range e.Discussion {
		inputs = append(inputs, textSource{text: item.Body, source: item.Source})
	}
	seen := map[string]struct{}{}
	out := []Reference{}
	for _, input := range inputs {
		for _, ref := range relatedwork.Extract(input.text, e.Thread.Ref.Repo) {
			if strings.EqualFold(ref.Repo.Owner, e.Thread.Ref.Repo.Owner) && strings.EqualFold(ref.Repo.Repo, e.Thread.Ref.Repo.Repo) && ref.Number == e.Thread.Ref.Number {
				continue
			}
			key := strings.ToLower(fmt.Sprintf("%s/%s:%s#%d", ref.Repo.Owner, ref.Repo.Repo, ref.Kind, ref.Number))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, Reference{
				Repo: ref.Repo, Kind: ref.Kind,
				Number: ref.Number, Source: input.source,
			})
			if len(out) == maxExplicitRefs {
				break
			}
		}
		if len(out) == maxExplicitRefs {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo.String() != out[j].Repo.String() {
			return out[i].Repo.String() < out[j].Repo.String()
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Number < out[j].Number
	})
	return out
}

func codeTerms(title string, labels []string) []string {
	inputs := append([]string{title}, labels...)
	seen := map[string]struct{}{}
	out := []string{}
	for _, input := range inputs {
		for _, token := range codeTokenPattern.FindAllString(input, -1) {
			key := strings.ToLower(token)
			if _, stop := codeStopWords[key]; stop {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, token)
			if len(out) == maxCodeTerms {
				return out
			}
		}
	}
	return out
}

func discussionCoverageGap(kind domain.ThreadKind, coverage []FacetCoverage) string {
	required := []string{"issue_comments"}
	if kind == domain.PullRequestKind {
		required = append(required, "pr_details", "pr_reviews", "pr_review_comments")
	}
	byFacet := map[string]FacetCoverage{}
	for _, item := range coverage {
		byFacet[item.Facet] = item
	}
	gaps := []string{}
	for _, facet := range required {
		item, ok := byFacet[facet]
		if !ok || !item.Present {
			gaps = append(gaps, facet+" not hydrated")
		} else if !item.Complete {
			gaps = append(gaps, facet+" incomplete")
		}
	}
	return strings.Join(gaps, "; ")
}

func discussionSources(e ThreadEvidence) []SourceRef {
	out := []SourceRef{e.Thread.Source}
	for _, facet := range e.Coverage {
		if facet.Present {
			out = append(out, facet.Source)
		}
	}
	for _, item := range e.Discussion {
		out = append(out, item.Source)
	}
	return normalizeSources(out)
}

func sourceMeta(sources []SourceRef, unknown string) SectionMeta {
	sources = normalizeSources(sources)
	status := StatusAvailable
	if unknown != "" {
		status = StatusPartial
		if len(sources) == 0 {
			status = StatusUnknown
		}
	}
	return SectionMeta{Status: status, Sources: sources, UnknownReason: unknown}
}

func normalizeSources(sources []SourceRef) []SourceRef {
	seen := map[string]struct{}{}
	out := make([]SourceRef, 0, len(sources))
	for _, source := range sources {
		if source.Source == "" && source.URL == "" && source.CommitSHA == "" {
			continue
		}
		source.ObservedAt = source.ObservedAt.UTC()
		source.AsOf = source.AsOf.UTC()
		key := source.Source + "|" + source.URL + "|" + source.CommitSHA + "|" + source.ObservedAt.Format(time.RFC3339Nano) + "|" + source.AsOf.Format(time.RFC3339Nano)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].URL != out[j].URL {
			return out[i].URL < out[j].URL
		}
		return out[i].CommitSHA < out[j].CommitSHA
	})
	return out
}

func fromDomainSources(sources []domain.SourceRef) []SourceRef {
	out := make([]SourceRef, 0, len(sources))
	for _, source := range sources {
		out = append(out, SourceRef{
			Source: source.Source, URL: source.URL, CommitSHA: source.CommitSHA,
			ObservedAt: source.ObservedAt, AsOf: source.AsOf,
		})
	}
	return normalizeSources(out)
}

func cleanSorted(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = value
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = seen[key]
	}
	return out
}

func boundedText(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\x00", ""))
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func eventTime(item DiscussionItem) time.Time {
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.UpdatedAt
}

func maintainerAssociation(association string) bool {
	switch strings.ToUpper(strings.TrimSpace(association)) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	default:
		return false
	}
}

func threadURL(ref ThreadRef) string {
	segment := "issues"
	if ref.Kind == domain.PullRequestKind {
		segment = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", ref.Repo, segment, ref.Number)
}

func latestSourceTime(sources []SourceRef) time.Time {
	var latest time.Time
	for _, source := range sources {
		candidate := source.AsOf
		if candidate.IsZero() {
			candidate = source.ObservedAt
		}
		if candidate.After(latest) {
			latest = candidate.UTC()
		}
	}
	return latest
}

func joinReasons(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "; " + right
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
