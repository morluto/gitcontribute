package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// MCPReader adapts Service to the mcpserver.Reader interface. It is a thin
// wrapper because Go does not allow two methods named Search on one type.
type MCPReader struct{ *Service }

// MCPReader returns an MCP adapter backed by this service. Read methods remain
// offline; methods named sync or hydrate are explicit network-read operations.
func (s *Service) MCPReader() mcpserver.Reader { return &MCPReader{s} }

// Search performs a local-only corpus search through the MCP interface.
func (r *MCPReader) Search(ctx context.Context, in mcpserver.SearchInput) (mcpserver.SearchOutput, error) {
	if in.Query == "" {
		return mcpserver.SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	var updatedAfter time.Time
	if strings.TrimSpace(in.UpdatedAfter) != "" {
		var err error
		updatedAfter, err = time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return mcpserver.SearchOutput{}, errors.New("updated_after must be RFC 3339")
		}
	}

	repo := ""
	if (in.Owner == "") != (in.Repo == "") {
		return mcpserver.SearchOutput{}, errors.New("owner and repo must be provided together")
	}
	if in.Owner != "" && in.Repo != "" {
		repo = in.Owner + "/" + in.Repo
	}
	res, err := r.Service.searchCorpus(ctx, in.Query, cli.SearchOptions{
		Kind:  in.Kind,
		Repo:  repo,
		State: in.State, StateReason: in.StateReason, Merged: in.Merged, Author: in.Author,
		Association: in.Association, Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: updatedAfter,
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
	if err != nil {
		return mcpserver.SearchOutput{}, err
	}

	matches := make([]mcpserver.ThreadOutput, len(res.Matches))
	for i, m := range res.Matches {
		updatedAt := ""
		if !m.UpdatedAt.IsZero() {
			updatedAt = m.UpdatedAt.Format(time.RFC3339)
		}
		matches[i] = mcpserver.ThreadOutput{
			Owner:             m.Repo.Owner,
			Repo:              m.Repo.Repo,
			Kind:              m.Kind,
			Number:            m.Number,
			State:             m.State,
			StateReason:       m.StateReason,
			Title:             m.Title,
			Body:              m.Body,
			Author:            m.Author,
			AuthorAssociation: m.AuthorAssociation,
			Labels:            m.Labels,
			Assignees:         m.Assignees,
			Draft:             m.Draft, ClosedAt: formatTime(m.ClosedAt), MergedAt: formatTime(m.MergedAt), Merged: knownMergePointer(m.Merged, m.MergedKnown),
			UpdatedAt:    updatedAt,
			MatchSource:  m.MatchSource,
			MatchExcerpt: m.MatchExcerpt,
		}
		if m.MatchSource != "" {
			matches[i].MatchUpdatedAt = formatTime(m.Freshness)
		}
	}
	return mcpserver.SearchOutput{Query: in.Query, Total: res.Total, Matches: matches, NextCursor: res.NextCursor}, nil
}

// Repository reads a repository projection from the local corpus.
func (r *MCPReader) Repository(ctx context.Context, in mcpserver.RepoInput) (mcpserver.RepositoryOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.RepositoryOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.RepositoryOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpserver.RepositoryOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.RepositoryOutput{}, mcpserver.ErrNotFound
	}
	return mcpserver.RepositoryOutput{
		Owner:     repo.Owner,
		Repo:      repo.Name,
		UpdatedAt: formatTime(repo.SourceUpdatedAt),
		Fields: map[string]any{
			"description":    repo.Description,
			"default_branch": repo.DefaultBranch,
			"language":       repo.Language,
			"license":        repo.License,
			"topics":         repo.Topics,
			"stars":          repo.Stars,
			"watchers":       repo.Watchers,
			"forks":          repo.Forks,
			"open_issues":    repo.OpenIssues,
			"archived":       repo.Archived,
			"fork":           repo.Fork,
		},
	}, nil
}

// Thread reads one issue or pull request from the local corpus.
func (r *MCPReader) Thread(ctx context.Context, in mcpserver.ThreadInput) (mcpserver.ThreadOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.ThreadOutput{}, err
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return mcpserver.ThreadOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Number < 1 {
		return mcpserver.ThreadOutput{}, errors.New("number must be positive")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.ThreadOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpserver.ThreadOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.ThreadOutput{}, mcpserver.ErrNotFound
	}
	thread, err := c.GetThread(ctx, repo.ID, in.Kind, in.Number)
	if err != nil {
		return mcpserver.ThreadOutput{}, fmt.Errorf("get thread: %w", err)
	}
	if thread == nil {
		return mcpserver.ThreadOutput{}, mcpserver.ErrNotFound
	}
	out := corpusThreadToMCPOutput(thread)
	out.Owner = in.Owner
	out.Repo = in.Repo
	return out, nil
}

func corpusThreadToMCPOutput(t *corpus.Thread) mcpserver.ThreadOutput {
	return mcpserver.ThreadOutput{
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

// Dossier builds a source-backed repository dossier from local corpus data.
func (r *MCPReader) Dossier(ctx context.Context, in mcpserver.RepoInput) (mcpserver.DossierOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.DossierOutput{}, err
	}
	if _, err := r.openReadOnlyCorpus(ctx); err != nil {
		return mcpserver.DossierOutput{}, err
	}
	d, err := r.Service.buildDossier(ctx, ref)
	if err != nil {
		return mcpserver.DossierOutput{}, err
	}
	return dossierToMCPOutput(d), nil
}

// SearchCode searches indexed code snapshots in the local corpus.
func (r *MCPReader) SearchCode(ctx context.Context, in mcpserver.SearchCodeInput) (mcpserver.SearchCodeOutput, error) {
	if in.Query == "" {
		return mcpserver.SearchCodeOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchCodeOutput{}, errors.New("limit must be between 1 and 100")
	}
	var ref domain.RepoRef
	if in.Owner != "" || in.Repo != "" {
		if (in.Owner == "") != (in.Repo == "") {
			return mcpserver.SearchCodeOutput{}, errors.New("owner and repo must be provided together")
		}
		ref = domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
		if err := ref.Validate(); err != nil {
			return mcpserver.SearchCodeOutput{}, err
		}
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.SearchCodeOutput{}, err
	}
	page, err := c.SearchCodeWithOptions(ctx, in.Query, corpus.CodeSearchOptions{Ref: ref, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return mcpserver.SearchCodeOutput{}, fmt.Errorf("search code: %w", err)
	}
	matches := page.Matches
	out := make([]mcpserver.CodeMatchOutput, len(matches))
	for i, m := range matches {
		repo := m.Repo.String()
		out[i] = mcpserver.CodeMatchOutput{
			ID:       fmt.Sprintf("%s@%s:%s", repo, m.Commit, m.Path),
			Repo:     repo,
			Commit:   m.Commit,
			Path:     m.Path,
			Language: m.Language,
			Snippet:  boundedText(m.Content, 2000),
			Bytes:    m.Bytes,
		}
	}
	return mcpserver.SearchCodeOutput{Query: in.Query, Total: page.Total, Matches: out, NextCursor: page.NextCursor}, nil
}

// Investigation reads a local investigation workspace from the corpus.
func (r *MCPReader) Investigation(ctx context.Context, in mcpserver.InvestigationInput) (mcpserver.InvestigationOutput, error) {
	id, err := normalizeMCPID("id", in.ID)
	if err != nil {
		return mcpserver.InvestigationOutput{}, err
	}
	in.ID = id
	if in.HypothesisLimit == 0 {
		in.HypothesisLimit = 20
	}
	if in.HypothesisLimit < 1 || in.HypothesisLimit > 100 {
		return mcpserver.InvestigationOutput{}, errors.New("hypothesis_limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.InvestigationOutput{}, err
	}
	inv, err := c.GetInvestigation(ctx, in.ID)
	if err != nil {
		if errors.Is(err, investigation.ErrNotFound) {
			return mcpserver.InvestigationOutput{}, mcpserver.ErrNotFound
		}
		return mcpserver.InvestigationOutput{}, fmt.Errorf("get investigation: %w", err)
	}
	if inv == nil {
		return mcpserver.InvestigationOutput{}, mcpserver.ErrNotFound
	}
	hypotheses, err := c.ListHypotheses(ctx, in.ID)
	if err != nil {
		return mcpserver.InvestigationOutput{}, fmt.Errorf("list hypotheses: %w", err)
	}
	hypothesisTotal := len(hypotheses)
	if len(hypotheses) > in.HypothesisLimit {
		hypotheses = hypotheses[:in.HypothesisLimit]
	}
	hyps := make([]mcpserver.HypothesisSummary, len(hypotheses))
	for i, h := range hypotheses {
		hyps[i] = mcpserver.HypothesisSummary{
			ID:          h.ID,
			Title:       h.Title,
			Category:    string(h.Category),
			Status:      string(h.Status),
			Description: h.Description,
		}
	}
	return mcpserver.InvestigationOutput{
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
func (r *MCPReader) ListOpportunities(ctx context.Context, in mcpserver.ListOpportunitiesInput) (mcpserver.ListOpportunitiesOutput, error) {
	id, err := normalizeMCPID("investigation_id", in.InvestigationID)
	if err != nil {
		return mcpserver.ListOpportunitiesOutput{}, err
	}
	in.InvestigationID = id
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.ListOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.ListOpportunitiesOutput{}, err
	}
	opps, err := c.ListOpportunities(ctx, in.InvestigationID)
	if err != nil {
		return mcpserver.ListOpportunitiesOutput{}, fmt.Errorf("list opportunities: %w", err)
	}
	total := len(opps)
	if len(opps) > in.Limit {
		opps = opps[:in.Limit]
	}
	out := make([]mcpserver.OpportunitySummary, len(opps))
	for i, o := range opps {
		out[i] = mcpserver.OpportunitySummary{
			ID:              o.ID,
			InvestigationID: o.InvestigationID,
			Title:           o.Title,
			Category:        string(o.Category),
			Status:          string(o.Status),
			Confidence:      o.Confidence,
			CollisionStatus: string(o.CollisionStatus),
			CreatedAt:       formatTime(o.CreatedAt),
			UpdatedAt:       formatTime(o.UpdatedAt),
		}
	}
	return mcpserver.ListOpportunitiesOutput{Opportunities: out, Total: total}, nil
}

// Opportunity reads a local contribution opportunity.
func (r *MCPReader) Opportunity(ctx context.Context, in mcpserver.OpportunityInput) (mcpserver.OpportunityOutput, error) {
	id, err := normalizeMCPID("id", in.ID)
	if err != nil {
		return mcpserver.OpportunityOutput{}, err
	}
	in.ID = id
	if in.EvidenceLimit == 0 {
		in.EvidenceLimit = 20
	}
	if in.EvidenceLimit < 1 || in.EvidenceLimit > 100 {
		return mcpserver.OpportunityOutput{}, errors.New("evidence_limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.OpportunityOutput{}, err
	}
	opp, err := c.GetOpportunity(ctx, in.ID)
	if err != nil {
		if errors.Is(err, investigation.ErrNotFound) {
			return mcpserver.OpportunityOutput{}, mcpserver.ErrNotFound
		}
		return mcpserver.OpportunityOutput{}, fmt.Errorf("get opportunity: %w", err)
	}
	if opp == nil {
		return mcpserver.OpportunityOutput{}, mcpserver.ErrNotFound
	}
	evs, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opp.ID})
	if err != nil {
		return mcpserver.OpportunityOutput{}, fmt.Errorf("list evidence: %w", err)
	}
	evidenceTotal := len(evs)
	if len(evs) > in.EvidenceLimit {
		evs = evs[:in.EvidenceLimit]
	}
	evidenceIDs := make([]string, len(evs))
	for i, e := range evs {
		evidenceIDs[i] = e.ID
	}
	return mcpserver.OpportunityOutput{
		ID:                  opp.ID,
		InvestigationID:     opp.InvestigationID,
		HypothesisID:        opp.HypothesisID,
		Title:               opp.Title,
		ProblemStatement:    opp.ProblemStatement,
		Category:            string(opp.Category),
		Scope:               opp.Scope,
		Impact:              opp.Impact,
		Confidence:          opp.Confidence,
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
func (r *MCPReader) Evidence(ctx context.Context, in mcpserver.EvidenceInput) (mcpserver.EvidenceOutput, error) {
	in.InvestigationID = strings.TrimSpace(in.InvestigationID)
	in.OpportunityID = strings.TrimSpace(in.OpportunityID)
	if (in.InvestigationID == "") == (in.OpportunityID == "") {
		return mcpserver.EvidenceOutput{}, errors.New("exactly one of investigation_id or opportunity_id is required")
	}
	if in.InvestigationID != "" {
		if _, err := normalizeMCPID("investigation_id", in.InvestigationID); err != nil {
			return mcpserver.EvidenceOutput{}, err
		}
	} else if _, err := normalizeMCPID("opportunity_id", in.OpportunityID); err != nil {
		return mcpserver.EvidenceOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.EvidenceOutput{}, errors.New("limit must be between 1 and 100")
	}
	filter := evidence.EvidenceFilter{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
	}
	if in.Relation != "" {
		if !isValidEvidenceRelation(in.Relation) {
			return mcpserver.EvidenceOutput{}, fmt.Errorf("invalid relation %q", in.Relation)
		}
		filter.Relation = evidence.Relation(in.Relation)
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.EvidenceOutput{}, err
	}
	items, err := c.ListEvidence(ctx, filter)
	if err != nil {
		return mcpserver.EvidenceOutput{}, fmt.Errorf("list evidence: %w", err)
	}
	total := len(items)
	if len(items) > in.Limit {
		items = items[:in.Limit]
	}
	out := make([]mcpserver.EvidenceItem, len(items))
	evaluator := evidence.NewFreshnessEvaluator(c)
	for i, e := range items {
		freshness, err := evaluator.Evaluate(ctx, e)
		if err != nil {
			return mcpserver.EvidenceOutput{}, fmt.Errorf("evaluate evidence %q: %w", e.ID, err)
		}
		out[i] = mcpserver.EvidenceItem{
			ID: e.ID, Type: string(e.Type), Relation: string(e.Relation), Description: e.Description,
			SourceRefs: sourceRefsToMCP(e.SourceRefs), SourceProvenance: evidenceSourceRevisionsToMCP(e.SourceProvenance),
			Freshness: string(freshness.Status), FreshnessReason: freshness.Reason, CreatedAt: formatTime(e.CreatedAt),
		}
	}
	return mcpserver.EvidenceOutput{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
		Total:           total,
		Evidence:        out,
	}, nil
}

func evidenceSourceRevisionsToMCP(values []evidence.SourceRevision) []mcpserver.EvidenceSourceRevision {
	if len(values) == 0 {
		return nil
	}
	out := make([]mcpserver.EvidenceSourceRevision, len(values))
	for i, value := range values {
		out[i] = mcpserver.EvidenceSourceRevision{
			Subject: mcpserver.EvidenceSourceSubject{
				Kind: string(value.Subject.Kind), Owner: value.Subject.Owner, Repo: value.Subject.Repo,
				ThreadKind: value.Subject.ThreadKind, Number: value.Subject.Number, Facet: value.Subject.Facet,
			},
			SourceUpdatedAt: formatTime(value.SourceUpdatedAt), ObservationSequence: value.ObservationSequence,
			ObservedAt: formatTime(value.ObservedAt),
		}
	}
	return out
}

func dossierToMCPOutput(d *domain.Dossier) mcpserver.DossierOutput {
	return mcpserver.DossierOutput{
		Owner: d.Repo.Owner,
		Repo:  d.Repo.Repo,
		AsOf:  d.AsOf.Format(time.RFC3339),
		Sections: map[string]any{
			"description":                     d.Repository.Description,
			"language":                        firstLanguage(d.Repository.Languages),
			"stars":                           d.Repository.Stars,
			"open_issues":                     d.OpenIssueCount,
			"closed_issues":                   d.ClosedIssueCount,
			"open_prs":                        d.OpenPullRequestCount,
			"merged_prs":                      d.MergedPullRequestCount,
			"closed_unmerged_prs":             d.ClosedUnmergedPullRequestCount,
			"closed_unknown_merge_prs":        d.ClosedPullRequestUnknownCount,
			"recent_merged_prs":               d.RecentMergedPullRequests,
			"recent_open_prs":                 d.RecentOpenPullRequests,
			"recent_closed_unmerged_prs":      d.RecentClosedUnmergedPullRequests,
			"recent_closed_unknown_merge_prs": d.RecentClosedUnknownPullRequests,
			"recent_issues":                   d.RecentIssues,
			"guidance":                        d.ContributionGuidance,
			"coverage":                        coverageNames(d.Coverage),
		},
	}
}

func sourceRefsToMCP(refs []domain.SourceRef) []mcpserver.SourceRef {
	out := make([]mcpserver.SourceRef, len(refs))
	for i, r := range refs {
		out[i] = mcpserver.SourceRef{
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

// FindClusters lists duplicate-candidate clusters for a repository from the
// local corpus without recomputing them.
func (r *MCPReader) FindClusters(ctx context.Context, in mcpserver.FindClustersInput) (mcpserver.FindClustersOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.FindClustersOutput{}, err
	}
	if in.Limit <= 0 || in.Limit > 100 {
		return mcpserver.FindClustersOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.FindClustersOutput{}, err
	}
	projection, err := c.ListClusterProjection(ctx, ref, clustering.ClusterOpen, in.Limit)
	if err != nil {
		return mcpserver.FindClustersOutput{}, fmt.Errorf("list clusters: %w", err)
	}
	out := mcpserver.FindClustersOutput{
		Owner:    in.Owner,
		Repo:     in.Repo,
		Total:    len(projection.Clusters),
		Clusters: make([]mcpserver.ClusterOutput, len(projection.Clusters)),
	}
	if projection.Projection != nil {
		out.RuleVersion = projection.Projection.RuleVersion
	}
	for i, cl := range projection.Clusters {
		out.Clusters[i] = clusterToMCP(cl, 20)
	}
	return out, nil
}

// FindNeighbors ranks similar local threads without network access.
func (r *MCPReader) FindNeighbors(ctx context.Context, in mcpserver.FindNeighborsInput) (mcpserver.FindNeighborsOutput, error) {
	result, err := r.Service.Neighbors(ctx, cli.RepoRef{Owner: in.Owner, Repo: in.Repo}, in.Kind, in.Number, in.Limit)
	if err != nil {
		return mcpserver.FindNeighborsOutput{}, err
	}
	out := mcpserver.FindNeighborsOutput{
		Owner: in.Owner, Repo: in.Repo, Kind: result.Kind, Number: result.Number, SourceRevision: result.SourceRevision,
		Neighbors: make([]mcpserver.NeighborOutput, len(result.Neighbors)),
	}
	for i, neighbor := range result.Neighbors {
		out.Neighbors[i] = mcpserver.NeighborOutput{
			Kind: neighbor.Kind, Owner: neighbor.Owner, Repo: neighbor.Repo, Number: neighbor.Number,
			Title: neighbor.Title, State: neighbor.State, Score: neighbor.Score, Reason: neighbor.Reason,
		}
	}
	return out, nil
}

// GetCoverage returns bounded, input-ordered facet coverage without network access.
func (r *MCPReader) GetCoverage(ctx context.Context, in mcpserver.GetCoverageInput) (mcpserver.GetCoverageOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 100 {
		return mcpserver.GetCoverageOutput{}, errors.New("targets must contain 1 to 100 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.GetCoverageOutput{}, err
	}
	out := mcpserver.GetCoverageOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.CoverageOutput], len(in.Targets))}
	for i, target := range in.Targets {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		key := coverageTargetKey(target)
		item := mcpserver.BatchItem[mcpserver.CoverageOutput]{Key: key, Status: "complete"}
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
			if reason == "not_indexed" {
				item.Message = "target is not present in the local corpus"
				item.NextAction = "Synchronize the repository or thread explicitly, then retry this item."
			} else {
				item.Message = "owner/repo and optional kind/number must identify a repository or exact thread"
			}
			out.Status = "partial"
		} else {
			item.Value = &value
		}
		out.Items[i] = item
	}
	return out, nil
}

func coverageTargetKey(target mcpserver.CoverageTarget) string {
	key := target.Owner + "/" + target.Repo
	if target.Kind != "" || target.Number != 0 {
		key += fmt.Sprintf("/%s#%d", target.Kind, target.Number)
	}
	return key
}

var errInvalidCoverageTarget = errors.New("invalid coverage target")

func readCoverageTarget(ctx context.Context, c *corpus.Corpus, target mcpserver.CoverageTarget) (mcpserver.CoverageOutput, string, error) {
	ref := domain.RepoRef{Owner: target.Owner, Repo: target.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.CoverageOutput{}, "invalid_reference", fmt.Errorf("%w: %w", errInvalidCoverageTarget, err)
	}
	isThread := target.Kind != "" || target.Number != 0
	if isThread && ((target.Kind != "issue" && target.Kind != "pull_request") || target.Number < 1) {
		return mcpserver.CoverageOutput{}, "invalid_reference", errInvalidCoverageTarget
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpserver.CoverageOutput{}, "", fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.CoverageOutput{}, "not_indexed", nil
	}
	var threadID *int64
	asOf := repo.SourceUpdatedAt
	if isThread {
		thread, err := c.GetThread(ctx, repo.ID, target.Kind, target.Number)
		if err != nil {
			return mcpserver.CoverageOutput{}, "", fmt.Errorf("get thread: %w", err)
		}
		if thread == nil {
			return mcpserver.CoverageOutput{}, "not_indexed", nil
		}
		threadID = &thread.ID
		asOf = thread.SourceUpdatedAt
	}
	covs, err := c.ListCoverage(ctx, repo.ID, threadID)
	if err != nil {
		return mcpserver.CoverageOutput{}, "", fmt.Errorf("list coverage: %w", err)
	}
	out := mcpserver.CoverageOutput{Owner: target.Owner, Repo: target.Repo, Kind: target.Kind, Number: target.Number, AsOf: formatTime(asOf), Facets: make([]mcpserver.FacetCoverageOutput, 0, len(covs))}
	for _, cov := range covs {
		if cov.SourceUpdatedAt.After(asOf) {
			asOf = cov.SourceUpdatedAt
			out.AsOf = formatTime(asOf)
		}
		status := "complete"
		if !cov.Complete {
			status = "incomplete"
		}
		out.Facets = append(out.Facets, mcpserver.FacetCoverageOutput{
			Facet:     cov.Facet,
			Complete:  cov.Complete,
			Status:    status,
			UpdatedAt: formatTime(cov.SourceUpdatedAt),
		})
	}
	return out, "", nil
}

// Lens reads a saved lens definition from the local corpus.
func (r *MCPReader) Lens(ctx context.Context, in mcpserver.LensInput) (mcpserver.LensOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return mcpserver.LensOutput{}, errors.New("name is required")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.LensOutput{}, err
	}
	record, err := c.GetLens(ctx, name)
	if err != nil {
		return mcpserver.LensOutput{}, fmt.Errorf("get lens: %w", err)
	}
	if record == nil {
		return mcpserver.LensOutput{}, mcpserver.ErrNotFound
	}
	return mcpserver.LensOutput{
		Name:       record.Definition.Name,
		Definition: record.Definition,
		CreatedAt:  formatTime(record.CreatedAt),
		UpdatedAt:  formatTime(record.UpdatedAt),
	}, nil
}

func clusterToMCP(cl clustering.Cluster, memberLimit int) mcpserver.ClusterOutput {
	members := make([]mcpserver.ClusterMemberOutput, 0, len(cl.Members))
	count := 0
	for _, m := range cl.Members {
		if memberLimit > 0 && count >= memberLimit {
			break
		}
		members = append(members, mcpserver.ClusterMemberOutput{
			Kind:     m.Ref.Kind,
			Owner:    m.Ref.Owner,
			Repo:     m.Ref.Repo,
			Number:   m.Ref.Number,
			Title:    m.Title,
			State:    m.State,
			Score:    m.Score,
			Reason:   m.Reason,
			Included: m.Included,
		})
		count++
	}
	return mcpserver.ClusterOutput{
		StableID:    cl.StableID,
		State:       string(cl.State),
		Canonical:   mcpserver.ClusterMemberOutput{Kind: cl.Canonical.Kind, Owner: cl.Canonical.Owner, Repo: cl.Canonical.Repo, Number: cl.Canonical.Number},
		MemberCount: len(cl.Members),
		Members:     members,
	}
}

// MCPRunner implements cli.MCPRunner by starting an MCP server over stdio.
type MCPRunner struct{ *Service }

// NewMCPRunner returns a cli.MCPRunner backed by the service.
func (s *Service) NewMCPRunner() cli.MCPRunner { return &MCPRunner{s} }

// Run starts the MCP server using the requested transport.
func (r *MCPRunner) Run(ctx context.Context, opts cli.MCPOptions) error {
	if opts.Transport != "stdio" {
		return fmt.Errorf("unsupported mcp transport %q", opts.Transport)
	}
	server, err := mcpserver.New(r.MCPReader(), r.version)
	if err != nil {
		return err
	}
	return server.ServeStdio(ctx)
}
