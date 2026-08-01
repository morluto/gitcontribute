package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// MCPReader adapts Service to the mcpcontract.Reader interface. It is a thin
// wrapper because Go does not allow two methods named Search on one type.
type MCPReader struct{ *Service }

// application disambiguates product methods shadowed by MCP projection methods
// with the same name.
func (r *MCPReader) application() *Service { return r.Service }

// MCPReader returns an MCP adapter backed by this service. Read methods remain
// offline; methods named sync or hydrate are explicit network-read operations.
func (s *Service) MCPReader() mcpcontract.Reader { return &MCPReader{s} }

// Search performs a local-only corpus search through the MCP interface.
func (r *MCPReader) Search(ctx context.Context, in mcpcontract.SearchInput) (mcpcontract.SearchOutput, error) {
	if in.Query == "" {
		return mcpcontract.SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MatchMode == "" {
		in.MatchMode = "all"
	}
	if in.MatchMode != "all" && in.MatchMode != "any" {
		return mcpcontract.SearchOutput{}, errors.New("match_mode must be all or any")
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return mcpcontract.SearchOutput{}, errors.New("view must be compact or full")
	}
	var updatedAfter time.Time
	if strings.TrimSpace(in.UpdatedAfter) != "" {
		var err error
		updatedAfter, err = time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return mcpcontract.SearchOutput{}, errors.New("updated_after must be RFC 3339")
		}
	}
	var updatedBefore time.Time
	if strings.TrimSpace(in.UpdatedBefore) != "" {
		var err error
		updatedBefore, err = time.Parse(time.RFC3339, in.UpdatedBefore)
		if err != nil {
			return mcpcontract.SearchOutput{}, errors.New("updated_before must be RFC 3339")
		}
	}
	if !updatedAfter.IsZero() && !updatedBefore.IsZero() && updatedBefore.Before(updatedAfter) {
		return mcpcontract.SearchOutput{}, errors.New("updated_before must not be earlier than updated_after")
	}

	repo := ""
	if (in.Owner == "") != (in.Repo == "") {
		return mcpcontract.SearchOutput{}, errors.New("owner and repo must be provided together")
	}
	if in.Owner != "" && in.Repo != "" {
		repo = in.Owner + "/" + in.Repo
	}
	res, err := r.searchCorpus(ctx, in.Query, contracts.SearchOptions{
		Kind:  in.Kind,
		Repo:  repo,
		State: in.State, StateReason: in.StateReason, Merged: in.Merged, Author: in.Author,
		Association: in.Association, Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: updatedAfter, UpdatedBefore: updatedBefore,
		Limit:  in.Limit,
		Cursor: in.Cursor,
		Sort:   in.Sort, MatchMode: in.MatchMode,
		SnapshotToken: in.SnapshotToken,
	})
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}

	matches := make([]mcpcontract.ThreadOutput, len(res.Matches))
	for i, m := range res.Matches {
		updatedAt := ""
		if !m.UpdatedAt.IsZero() {
			updatedAt = m.UpdatedAt.Format(time.RFC3339)
		}
		matches[i] = mcpcontract.ThreadOutput{
			Owner:             m.Repo.Owner,
			Repo:              m.Repo.Repo,
			Kind:              m.Kind,
			Number:            m.Number,
			State:             m.State,
			StateReason:       m.StateReason,
			Title:             m.Title,
			Body:              "",
			Author:            m.Author,
			AuthorAssociation: m.AuthorAssociation,
			Labels:            m.Labels,
			Assignees:         m.Assignees,
			Draft:             m.Draft, ClosedAt: formatTime(m.ClosedAt), MergedAt: formatTime(m.MergedAt), Merged: knownMergePointer(m.Merged, m.MergedKnown),
			UpdatedAt:      updatedAt,
			MatchSource:    m.MatchSource,
			MatchExcerpt:   m.MatchExcerpt,
			MatchTruncated: m.MatchTruncated,
			SnapshotToken:  res.SnapshotToken,
		}
		if in.View == "full" {
			matches[i].Body = m.Body
		}
		if m.MatchSource != "" {
			matches[i].MatchUpdatedAt = formatTime(m.Freshness)
		}
	}
	separator := " AND "
	if in.MatchMode == "any" {
		separator = " OR "
	}
	out := mcpcontract.SearchOutput{
		Query: in.Query, QueryInterpretation: strings.Join(strings.Fields(in.Query), separator),
		MatchMode: in.MatchMode, View: in.View, Total: res.Total, Matches: matches, NextCursor: res.NextCursor,
		UnknownMergeCount: res.UnknownMergeCount,
		SnapshotToken:     res.SnapshotToken,
	}
	provenance, err := offlineReadProvenance("thread_search", res.ObservationWatermark, in, res.NextCursor == "", res.NextCursor != "", true)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}
	out.Provenance = provenance
	if provenance.UnknownCoverage {
		out.Recovery = localThreadSearchRecovery(in)
		out.Provenance.Recovery = out.Recovery
	}
	if out.UnknownMergeCount > 0 {
		out.Suggestion = "Some otherwise-matching pull requests have unknown merge state. Repeat without the merged filter to identify finalists, then hydrate pr_details before inferring absence."
	} else if out.Total == 0 && in.MatchMode == "all" && len(strings.Fields(in.Query)) > 1 {
		out.Suggestion = "No all-term matches. Retry with match_mode=any or fewer terms; verify corpus coverage before inferring absence."
	}
	return out, nil
}

// Repository reads a repository projection from the local corpus.
func (r *MCPReader) Repository(ctx context.Context, in mcpcontract.RepoInput) (mcpcontract.RepositoryOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.RepositoryOutput{}, err
	}
	batch, err := r.GetRepositories(ctx, mcpcontract.GetRepositoriesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner, Repo: ref.Repo}},
	})
	if err != nil {
		return mcpcontract.RepositoryOutput{}, err
	}
	if len(batch.Items) != 1 || batch.Items[0].Value == nil {
		return mcpcontract.RepositoryOutput{}, failure.NotFound(nil)
	}
	return *batch.Items[0].Value, nil
}

// Thread reads one issue or pull request from the local corpus.
func (r *MCPReader) Thread(ctx context.Context, in mcpcontract.ThreadInput) (mcpcontract.ThreadOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return mcpcontract.ThreadOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Number < 1 {
		return mcpcontract.ThreadOutput{}, errors.New("number must be positive")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpcontract.ThreadOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpcontract.ThreadOutput{}, failure.NotFound(nil)
	}
	thread, err := c.GetThread(ctx, repo.ID, in.Kind, in.Number)
	if err != nil {
		return mcpcontract.ThreadOutput{}, fmt.Errorf("get thread: %w", err)
	}
	if thread == nil {
		return mcpcontract.ThreadOutput{}, failure.NotFound(nil)
	}
	out := corpusThreadToMCPOutput(thread)
	out.Owner = in.Owner
	out.Repo = in.Repo
	out.SnapshotToken = snapshotIdentity(in.SnapshotToken, revision)
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	return out, nil
}

func corpusThreadToMCPOutput(t *corpus.Thread) mcpcontract.ThreadOutput {
	return mcpcontract.ThreadOutput{
		Owner:             "", // filled by caller
		Repo:              "",
		Kind:              t.Kind,
		Number:            t.Number,
		State:             t.State,
		StateReason:       t.StateReason,
		Title:             t.Title,
		Body:              t.Body,
		Author:            t.Author,
		AuthorAssociation: t.AuthorAssociation,
		Labels:            t.Labels,
		Assignees:         t.Assignees,
		Draft:             t.Draft, ClosedAt: formatTime(t.ClosedAt), MergedAt: formatTime(t.MergedAt), Merged: knownMergePointer(t.Merged, t.MergedKnown),
		UpdatedAt: formatTime(t.SourceUpdatedAt),
	}
}

func knownMergePointer(merged, known bool) *bool {
	if !known {
		return nil
	}
	return &merged
}

// Dossier returns the latest persisted source-backed repository dossier.
func (r *MCPReader) Dossier(ctx context.Context, in mcpcontract.RepoInput) (mcpcontract.DossierOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.DossierOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.DossierOutput{}, err
	}
	repository, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.DossierOutput{}, err
	}
	if repository == nil {
		return mcpcontract.DossierOutput{}, mcpcontract.Unavailable(
			"repository_not_indexed",
			fmt.Sprintf("Repository %s is not present in the local corpus.", ref),
			mcpcontract.RecoveryAction(mcpcontract.SyncRepositoryContextInput{Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner, Repo: ref.Repo}}}),
		)
	}
	record, sources, err := c.GetDossier(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.DossierOutput{}, fmt.Errorf("get dossier: %w", err)
	}
	if record == nil {
		return mcpcontract.DossierOutput{}, mcpcontract.Unavailable(
			"dossier_not_persisted",
			fmt.Sprintf("No persisted dossier exists for %s.", ref),
			mcpcontract.RecoveryAction(mcpcontract.GetRepositoriesInput{Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner, Repo: ref.Repo}}}),
		)
	}
	d, err := dossierFromRecord(record, sources)
	if err != nil {
		return mcpcontract.DossierOutput{}, err
	}
	return dossierToMCPOutput(d), nil
}

// Investigation reads a local investigation workspace from the corpus.
func (r *MCPReader) Investigation(ctx context.Context, in mcpcontract.InvestigationInput) (mcpcontract.InvestigationOutput, error) {
	id, err := normalizeMCPID("id", in.ID)
	if err != nil {
		return mcpcontract.InvestigationOutput{}, err
	}
	in.ID = id
	if in.HypothesisLimit == 0 {
		in.HypothesisLimit = 20
	}
	if in.HypothesisLimit < 1 || in.HypothesisLimit > 100 {
		return mcpcontract.InvestigationOutput{}, errors.New("hypothesis_limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.InvestigationOutput{}, err
	}
	inv, err := c.GetInvestigation(ctx, in.ID)
	if err != nil {
		if errors.Is(err, investigation.ErrNotFound) {
			return mcpcontract.InvestigationOutput{}, failure.NotFound(nil)
		}
		return mcpcontract.InvestigationOutput{}, fmt.Errorf("get investigation: %w", err)
	}
	if inv == nil {
		return mcpcontract.InvestigationOutput{}, failure.NotFound(nil)
	}
	hypotheses, err := c.ListHypotheses(ctx, in.ID)
	if err != nil {
		return mcpcontract.InvestigationOutput{}, fmt.Errorf("list hypotheses: %w", err)
	}
	hypothesisTotal := len(hypotheses)
	if len(hypotheses) > in.HypothesisLimit {
		hypotheses = hypotheses[:in.HypothesisLimit]
	}
	hyps := make([]mcpcontract.HypothesisSummary, len(hypotheses))
	for i, h := range hypotheses {
		hyps[i] = mcpcontract.HypothesisSummary{
			ID:          h.ID,
			Title:       h.Title,
			Category:    string(h.Category),
			Status:      string(h.Status),
			Description: h.Description,
		}
	}
	return mcpcontract.InvestigationOutput{
		ID:              inv.ID,
		Owner:           inv.Repo.Owner,
		Repo:            inv.Repo.Repo,
		CommitSHA:       inv.CommitSHA,
		Lens:            inv.Lens,
		Status:          string(inv.Status),
		CreatedAt:       formatTime(inv.CreatedAt),
		UpdatedAt:       formatTime(inv.UpdatedAt),
		HypothesisTotal: hypothesisTotal,
		Hypotheses:      hyps,
	}, nil
}

// ListOpportunities lists opportunities for a local investigation.
func (r *MCPReader) ListOpportunities(ctx context.Context, in mcpcontract.ListOpportunitiesInput) (mcpcontract.ListOpportunitiesOutput, error) {
	id, err := normalizeMCPID("investigation_id", in.InvestigationID)
	if err != nil {
		return mcpcontract.ListOpportunitiesOutput{}, err
	}
	in.InvestigationID = id
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.ListOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ListOpportunitiesOutput{}, err
	}
	opps, err := c.ListOpportunities(ctx, in.InvestigationID)
	if err != nil {
		return mcpcontract.ListOpportunitiesOutput{}, fmt.Errorf("list opportunities: %w", err)
	}
	total := len(opps)
	if len(opps) > in.Limit {
		opps = opps[:in.Limit]
	}
	out := make([]mcpcontract.OpportunitySummary, len(opps))
	for i, o := range opps {
		out[i] = mcpcontract.OpportunitySummary{
			ID:              o.ID,
			InvestigationID: o.InvestigationID,
			Title:           o.Title,
			Category:        string(o.Category),
			Status:          string(o.Status),
			Confidence:      mcpcontract.Probability(o.Confidence),
			CollisionStatus: string(o.CollisionStatus),
			CreatedAt:       formatTime(o.CreatedAt),
			UpdatedAt:       formatTime(o.UpdatedAt),
		}
	}
	return mcpcontract.ListOpportunitiesOutput{Opportunities: out, Total: total}, nil
}

// Opportunity reads a local contribution opportunity.
func (r *MCPReader) Opportunity(ctx context.Context, in mcpcontract.OpportunityInput) (mcpcontract.OpportunityOutput, error) {
	id, err := normalizeMCPID("id", in.ID)
	if err != nil {
		return mcpcontract.OpportunityOutput{}, err
	}
	in.ID = id
	if in.EvidenceLimit == 0 {
		in.EvidenceLimit = 20
	}
	if in.EvidenceLimit < 1 || in.EvidenceLimit > 100 {
		return mcpcontract.OpportunityOutput{}, errors.New("evidence_limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.OpportunityOutput{}, err
	}
	opp, err := c.GetOpportunity(ctx, in.ID)
	if err != nil {
		if errors.Is(err, investigation.ErrNotFound) {
			return mcpcontract.OpportunityOutput{}, failure.NotFound(nil)
		}
		return mcpcontract.OpportunityOutput{}, fmt.Errorf("get opportunity: %w", err)
	}
	if opp == nil {
		return mcpcontract.OpportunityOutput{}, failure.NotFound(nil)
	}
	evs, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opp.ID})
	if err != nil {
		return mcpcontract.OpportunityOutput{}, fmt.Errorf("list evidence: %w", err)
	}
	evidenceTotal := len(evs)
	if len(evs) > in.EvidenceLimit {
		evs = evs[:in.EvidenceLimit]
	}
	evidenceIDs := make([]string, len(evs))
	for i, e := range evs {
		evidenceIDs[i] = e.ID
	}
	return mcpcontract.OpportunityOutput{
		ID:                  opp.ID,
		InvestigationID:     opp.InvestigationID,
		HypothesisID:        opp.HypothesisID,
		Title:               opp.Title,
		ProblemStatement:    opp.ProblemStatement,
		Category:            string(opp.Category),
		Scope:               opp.Scope,
		Impact:              opp.Impact,
		Confidence:          mcpcontract.Probability(opp.Confidence),
		ExpectedEffort:      opp.ExpectedEffort,
		Dependencies:        opp.Dependencies,
		CollisionStatus:     string(opp.CollisionStatus),
		MaintainerAlignment: opp.MaintainerAlignment,
		SourceRefs:          sourceRefsToMCP(opp.SourceRefs),
		EvidenceTotal:       evidenceTotal,
		EvidenceIDs:         evidenceIDs,
		Status:              string(opp.Status),
		CreatedAt:           formatTime(opp.CreatedAt),
		UpdatedAt:           formatTime(opp.UpdatedAt),
	}, nil
}

// Evidence reads evidence for a local investigation or opportunity.
func (r *MCPReader) Evidence(ctx context.Context, in mcpcontract.EvidenceInput) (mcpcontract.EvidenceOutput, error) {
	in.InvestigationID = strings.TrimSpace(in.InvestigationID)
	in.OpportunityID = strings.TrimSpace(in.OpportunityID)
	if (in.InvestigationID == "") == (in.OpportunityID == "") {
		return mcpcontract.EvidenceOutput{}, errors.New("exactly one of investigation_id or opportunity_id is required")
	}
	if in.InvestigationID != "" {
		if _, err := normalizeMCPID("investigation_id", in.InvestigationID); err != nil {
			return mcpcontract.EvidenceOutput{}, err
		}
	} else if _, err := normalizeMCPID("opportunity_id", in.OpportunityID); err != nil {
		return mcpcontract.EvidenceOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.EvidenceOutput{}, errors.New("limit must be between 1 and 100")
	}
	filter := evidence.EvidenceFilter{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
	}
	if in.Relation != "" {
		if !isValidEvidenceRelation(in.Relation) {
			return mcpcontract.EvidenceOutput{}, fmt.Errorf("invalid relation %q", in.Relation)
		}
		filter.Relation = evidence.Relation(in.Relation)
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.EvidenceOutput{}, err
	}
	items, err := c.ListEvidence(ctx, filter)
	if err != nil {
		return mcpcontract.EvidenceOutput{}, fmt.Errorf("list evidence: %w", err)
	}
	total := len(items)
	if len(items) > in.Limit {
		items = items[:in.Limit]
	}
	out := make([]mcpcontract.EvidenceItem, len(items))
	evaluator := evidence.NewFreshnessEvaluator(c)
	for i, e := range items {
		freshness, err := evaluator.Evaluate(ctx, e)
		if err != nil {
			return mcpcontract.EvidenceOutput{}, fmt.Errorf("evaluate evidence %q: %w", e.ID, err)
		}
		out[i] = mcpcontract.EvidenceItem{
			ID: e.ID, Type: string(e.Type), Relation: string(e.Relation), Description: e.Description,
			SourceRefs: sourceRefsToMCP(e.SourceRefs), SourceProvenance: evidenceSourceRevisionsToMCP(e.SourceProvenance),
			Freshness: string(freshness.Status), FreshnessReason: freshness.Reason, CreatedAt: formatTime(e.CreatedAt),
		}
	}
	return mcpcontract.EvidenceOutput{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
		Total:           total,
		Evidence:        out,
	}, nil
}

func evidenceSourceRevisionsToMCP(values []evidence.SourceRevision) []mcpcontract.EvidenceSourceRevision {
	if len(values) == 0 {
		return nil
	}
	out := make([]mcpcontract.EvidenceSourceRevision, len(values))
	for i, value := range values {
		out[i] = mcpcontract.EvidenceSourceRevision{
			Subject: mcpcontract.EvidenceSourceSubject{
				Kind: string(value.Subject.Kind), Owner: value.Subject.Owner, Repo: value.Subject.Repo,
				ThreadKind: value.Subject.ThreadKind, Number: value.Subject.Number, Facet: value.Subject.Facet,
			},
			SourceUpdatedAt: formatTime(value.SourceUpdatedAt), ObservationSequence: value.ObservationSequence,
			ObservedAt: formatTime(value.ObservedAt),
		}
	}
	return out
}

func dossierToMCPOutput(d *domain.Dossier) mcpcontract.DossierOutput {
	recentLimit := max(
		len(d.RecentMergedPullRequests),
		len(d.RecentOpenPullRequests),
		len(d.RecentClosedUnmergedPullRequests),
		len(d.RecentClosedUnknownPullRequests),
		len(d.RecentIssues),
	)
	recentTruncated :=
		d.MergedPullRequestCount > len(d.RecentMergedPullRequests) ||
			d.OpenPullRequestCount > len(d.RecentOpenPullRequests) ||
			d.ClosedUnmergedPullRequestCount > len(d.RecentClosedUnmergedPullRequests) ||
			d.ClosedPullRequestUnknownCount > len(d.RecentClosedUnknownPullRequests) ||
			d.OpenIssueCount+d.ClosedIssueCount > len(d.RecentIssues)
	return mcpcontract.DossierOutput{
		Owner: d.Repo.Owner, Repo: d.Repo.Repo, AsOf: d.AsOf.Format(time.RFC3339),
		RecentItemsLimit: mcpcontract.NonNegativeInt(recentLimit), RecentItemsTruncated: recentTruncated,
		Sections: mcpcontract.DossierSections{
			Description: d.Repository.Description, Language: firstLanguage(d.Repository.Languages),
			Stars:      mcpcontract.NonNegativeInt(d.Repository.Stars),
			OpenIssues: mcpcontract.NonNegativeInt(d.OpenIssueCount), ClosedIssues: mcpcontract.NonNegativeInt(d.ClosedIssueCount),
			OpenPullRequests: mcpcontract.NonNegativeInt(d.OpenPullRequestCount), MergedPullRequests: mcpcontract.NonNegativeInt(d.MergedPullRequestCount),
			ClosedUnmergedPullRequests:       mcpcontract.NonNegativeInt(d.ClosedUnmergedPullRequestCount),
			ClosedUnknownMergePullRequests:   mcpcontract.NonNegativeInt(d.ClosedPullRequestUnknownCount),
			RecentMergedPullRequests:         dossierThreadsToMCP(d.RecentMergedPullRequests),
			RecentOpenPullRequests:           dossierThreadsToMCP(d.RecentOpenPullRequests),
			RecentClosedUnmergedPullRequests: dossierThreadsToMCP(d.RecentClosedUnmergedPullRequests),
			RecentClosedUnknownPullRequests:  dossierThreadsToMCP(d.RecentClosedUnknownPullRequests),
			RecentIssues:                     dossierThreadsToMCP(d.RecentIssues),
			Guidance:                         d.ContributionGuidance, Coverage: coverageNames(d.Coverage),
		},
	}
}

func dossierThreadsToMCP(threads []domain.DossierThread) []mcpcontract.DossierThreadOutput {
	out := make([]mcpcontract.DossierThreadOutput, len(threads))
	for i, thread := range threads {
		out[i] = mcpcontract.DossierThreadOutput{
			Number: thread.Number, Title: thread.Title, Author: thread.Author, State: string(thread.State), Draft: thread.Draft,
			CreatedAt: formatTime(thread.CreatedAt), UpdatedAt: formatTime(thread.UpdatedAt),
			ClosedAt: formatTime(thread.ClosedAt), MergedAt: formatTime(thread.MergedAt),
			Labels: append([]string(nil), thread.Labels...),
		}
	}
	return out
}

func sourceRefsToMCP(refs []domain.SourceRef) []mcpcontract.SourceRef {
	out := make([]mcpcontract.SourceRef, len(refs))
	for i, r := range refs {
		out[i] = mcpcontract.SourceRef{
			Source:     r.Source,
			URL:        r.URL,
			CommitSHA:  r.CommitSHA,
			ObservedAt: formatTime(r.ObservedAt),
			AsOf:       formatTime(r.AsOf),
		}
	}
	return out
}

func isValidEvidenceRelation(s string) bool {
	switch evidence.Relation(s) {
	case evidence.RelationSupporting, evidence.RelationContradicting, evidence.RelationInconclusive, evidence.RelationStale, evidence.RelationInvalid:
		return true
	}
	return false
}

func normalizeMCPID(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if len(value) > 128 {
		return "", fmt.Errorf("%s exceeds 128 bytes", field)
	}
	return value, nil
}

// GetCoverage returns bounded, input-ordered facet coverage without network access.
func (r *MCPReader) GetCoverage(ctx context.Context, in mcpcontract.GetCoverageInput) (mcpcontract.GetCoverageOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 100 {
		return mcpcontract.GetCoverageOutput{}, errors.New("targets must contain 1 to 100 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetCoverageOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetCoverageOutput{}, err
	}
	out := mcpcontract.GetCoverageOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.CoverageOutput], len(in.Targets)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	for i, target := range in.Targets {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		key := coverageTargetKey(target)
		item := mcpcontract.BatchItem[mcpcontract.CoverageOutput]{Key: key, Status: "complete"}
		value, reason, err := readCoverageTarget(ctx, c, target)
		if errors.Is(err, errInvalidCoverageTarget) {
			item.Status, item.Reason = "unavailable", "invalid_reference"
			item.Message = "owner/repo and optional kind/number must identify a repository or exact thread"
			out.Status = "partial"
		} else if err != nil {
			item.Status, item.Reason, item.Message = "failed", "read_failed", err.Error()
			out.Status = "partial"
		} else if reason != "" {
			item.Status, item.Reason = "unavailable", reason
			item.Message = "owner/repo and optional kind/number must identify a repository or exact thread"
			switch reason {
			case "repository_not_indexed":
				item.Message = "target is not present in the local corpus"
				item.Recovery = recoveryPlan(reason, item.Message, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
			case "thread_not_indexed":
				item.Message = "target is not present in the local corpus"
				item.Recovery = recoveryPlan(reason, item.Message, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
			}
			out.Status = "partial"
		} else {
			value = withExpectedCoverageFacets(target, value)
			item.Value = &value
			if coverageNeedsRecovery(value) {
				item.Status = "retryable"
				item.Reason = "coverage_incomplete"
				item.Message = "one or more required coverage facets are missing or incomplete"
				item.Recovery = coverageRecoveryPlan(target, value)
				out.Status = "partial"
			}
		}
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.GetCoverageOutput{}, err
	}
	unknownCoverage := out.Status != "complete"
	for _, item := range out.Items {
		if item.Value == nil {
			unknownCoverage = true
			continue
		}
		for _, facet := range item.Value.Facets {
			if !facet.Complete {
				unknownCoverage = true
			}
		}
	}
	out.Provenance, err = offlineReadProvenance("coverage", revision, in, !unknownCoverage, false, unknownCoverage)
	if err != nil {
		return mcpcontract.GetCoverageOutput{}, err
	}
	return out, nil
}

func coverageTargetKey(target mcpcontract.CoverageTarget) string {
	key := target.Repository.Owner + "/" + target.Repository.Repo
	if target.Type == mcpcontract.CoverageTargetExactThread && target.Thread != nil {
		key += fmt.Sprintf("/%s#%d", target.Thread.Kind, target.Thread.Number)
	}
	return key
}

var errInvalidCoverageTarget = errors.New("invalid coverage target")

func readCoverageTarget(ctx context.Context, c *corpus.Corpus, target mcpcontract.CoverageTarget) (mcpcontract.CoverageOutput, string, error) {
	ref := domain.RepoRef{Owner: target.Repository.Owner, Repo: target.Repository.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.CoverageOutput{}, "invalid_reference", fmt.Errorf("%w: %w", errInvalidCoverageTarget, err)
	}
	isThread := target.Type == mcpcontract.CoverageTargetExactThread
	valid := target.Type == mcpcontract.CoverageTargetRepository && target.Thread == nil
	if isThread && target.Thread != nil {
		valid = (target.Thread.Kind == "issue" || target.Thread.Kind == "pull_request") && target.Thread.Number > 0
	}
	if !valid {
		return mcpcontract.CoverageOutput{}, "invalid_reference", errInvalidCoverageTarget
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.CoverageOutput{}, "", fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpcontract.CoverageOutput{}, "repository_not_indexed", nil
	}
	var threadID *int64
	asOf := repo.SourceUpdatedAt
	if isThread {
		thread, err := c.GetThread(ctx, repo.ID, target.Thread.Kind, target.Thread.Number)
		if err != nil {
			return mcpcontract.CoverageOutput{}, "", fmt.Errorf("get thread: %w", err)
		}
		if thread == nil {
			return mcpcontract.CoverageOutput{}, "thread_not_indexed", nil
		}
		threadID = &thread.ID
		asOf = thread.SourceUpdatedAt
	}
	covs, err := c.ListCoverage(ctx, repo.ID, threadID)
	if err != nil {
		return mcpcontract.CoverageOutput{}, "", fmt.Errorf("list coverage: %w", err)
	}
	out := mcpcontract.CoverageOutput{Owner: target.Repository.Owner, Repo: target.Repository.Repo, AsOf: formatTime(asOf), Facets: make([]mcpcontract.FacetCoverageOutput, 0, len(covs))}
	if target.Thread != nil {
		out.Kind, out.Number = target.Thread.Kind, target.Thread.Number
	}
	for _, cov := range covs {
		if cov.SourceUpdatedAt.After(asOf) {
			asOf = cov.SourceUpdatedAt
			out.AsOf = formatTime(asOf)
		}
		status := "complete"
		if !cov.Complete {
			status = "incomplete"
		}
		out.Facets = append(out.Facets, mcpcontract.FacetCoverageOutput{
			Facet:     cov.Facet,
			Complete:  cov.Complete,
			Status:    status,
			UpdatedAt: formatTime(cov.SourceUpdatedAt),
		})
	}
	return out, "", nil
}

func withExpectedCoverageFacets(target mcpcontract.CoverageTarget, value mcpcontract.CoverageOutput) mcpcontract.CoverageOutput {
	byFacet := make(map[string]struct{}, len(value.Facets))
	for _, facet := range value.Facets {
		byFacet[facet.Facet] = struct{}{}
	}
	for _, name := range coverageFacetNames(target) {
		if _, ok := byFacet[name]; ok {
			continue
		}
		value.Facets = append(value.Facets, mcpcontract.FacetCoverageOutput{Facet: name, Status: "unknown"})
	}
	return value
}

func coverageFacetNames(target mcpcontract.CoverageTarget) []string {
	if target.Type == mcpcontract.CoverageTargetRepository {
		return []string{"metadata", "threads", FacetContributionGuidance}
	}
	if target.Thread == nil {
		return nil
	}
	return facets.DefaultFor(target.Thread.Kind)
}

func coverageNeedsRecovery(value mcpcontract.CoverageOutput) bool {
	if len(value.Facets) == 0 {
		return true
	}
	for _, facet := range value.Facets {
		if !facet.Complete {
			return true
		}
	}
	return false
}

func coverageRecoveryPlan(target mcpcontract.CoverageTarget, value mcpcontract.CoverageOutput) *mcpcontract.RecoveryPlan {
	message := "Refresh the missing or incomplete coverage facets, then reread corpus.get_coverage."
	if target.Type == mcpcontract.CoverageTargetRepository {
		return recoveryPlan("coverage_incomplete", message, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
	}
	if target.Thread == nil {
		return recoveryPlan("coverage_incomplete", message, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
	}

	ref := mcpcontract.ThreadRef{Owner: target.Repository.Owner, Repo: target.Repository.Repo, Kind: target.Thread.Kind, Number: target.Thread.Number}
	selectable := make(map[string]struct{}, len(facets.SelectableFor(target.Thread.Kind)))
	for _, name := range facets.SelectableFor(target.Thread.Kind) {
		selectable[name] = struct{}{}
	}
	known := make(map[string]struct{}, len(facets.AllNames()))
	for _, name := range facets.AllNames() {
		known[name] = struct{}{}
	}
	selected := make([]string, 0, len(value.Facets))
	var additional []mcpcontract.ToolCall
	needsEnsure := false
	for _, facet := range value.Facets {
		if facet.Complete {
			continue
		}
		if _, ok := selectable[facet.Facet]; ok {
			selected = append(selected, facet.Facet)
			continue
		}
		if _, ok := known[facet.Facet]; ok {
			additional = append(additional, syncFacetCall(ref, facet.Facet))
			continue
		}
		needsEnsure = true
	}
	if len(selected) > 0 {
		calls := []mcpcontract.ToolCall{syncThreadFacetsCall(ref, selected)}
		calls = append(calls, additional...)
		if needsEnsure {
			calls = append(calls, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
		}
		return recoveryPlan("coverage_incomplete", message, calls...)
	}
	if len(additional) > 0 && !needsEnsure {
		return recoveryPlan("coverage_incomplete", message, additional...)
	}
	return recoveryPlan("coverage_incomplete", message, mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: target}))
}

// Lens reads a saved lens definition from the local corpus.
func (r *MCPReader) Lens(ctx context.Context, in mcpcontract.LensInput) (mcpcontract.LensOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return mcpcontract.LensOutput{}, errors.New("name is required")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.LensOutput{}, err
	}
	record, err := c.GetLens(ctx, name)
	if err != nil {
		return mcpcontract.LensOutput{}, fmt.Errorf("get lens: %w", err)
	}
	if record == nil {
		return mcpcontract.LensOutput{}, failure.NotFound(nil)
	}
	return mcpcontract.LensOutput{
		Name:       record.Definition.Name,
		Definition: record.Definition,
		CreatedAt:  formatTime(record.CreatedAt),
		UpdatedAt:  formatTime(record.UpdatedAt),
	}, nil
}

func clusterToMCP(cl clustering.Cluster, memberLimit int) mcpcontract.ClusterOutput {
	members := make([]mcpcontract.ClusterMemberOutput, 0, len(cl.Members))
	count := 0
	for _, m := range cl.Members {
		if memberLimit > 0 && count >= memberLimit {
			break
		}
		members = append(members, mcpcontract.ClusterMemberOutput{
			Kind:     m.Ref.Kind,
			Owner:    m.Ref.Owner,
			Repo:     m.Ref.Repo,
			Number:   m.Ref.Number,
			Title:    m.Title,
			State:    m.State,
			Score:    mcpcontract.SimilarityScore(m.Score),
			Reason:   m.Reason,
			Included: m.Included,
		})
		count++
	}
	return mcpcontract.ClusterOutput{
		StableID:    cl.StableID,
		State:       string(cl.State),
		Canonical:   mcpcontract.ClusterMemberOutput{Kind: cl.Canonical.Kind, Owner: cl.Canonical.Owner, Repo: cl.Canonical.Repo, Number: cl.Canonical.Number},
		MemberCount: len(cl.Members),
		Members:     members,
	}
}
