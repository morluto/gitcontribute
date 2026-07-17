package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// MCPReader adapts Service to the mcpserver.Reader interface. It is a thin
// wrapper because Go does not allow two methods named Search on one type.
type MCPReader struct{ *Service }

// MCPReader returns a local, read-only MCP reader backed by this service.
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
		Limit: in.Limit,
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
			Owner:     m.Repo.Owner,
			Repo:      m.Repo.Repo,
			Kind:      m.Kind,
			Number:    m.Number,
			State:     m.State,
			Title:     m.Title,
			Body:      m.Body,
			Author:    m.Author,
			Labels:    m.Labels,
			UpdatedAt: updatedAt,
		}
	}
	return mcpserver.SearchOutput{Query: in.Query, Total: res.Total, Matches: matches}, nil
}

// Repository reads a repository projection from the local corpus.
func (r *MCPReader) Repository(ctx context.Context, in mcpserver.RepoInput) (mcpserver.RepositoryOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.RepositoryOutput{}, err
	}
	c, err := r.Service.openCorpus(ctx)
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
		UpdatedAt: repo.SourceUpdatedAt.Format(time.RFC3339),
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
	c, err := r.Service.openCorpus(ctx)
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
	updatedAt := ""
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.Format(time.RFC3339)
	}
	return mcpserver.ThreadOutput{
		Owner:     "", // filled by caller
		Repo:      "",
		Kind:      t.Kind,
		Number:    t.Number,
		State:     t.State,
		Title:     t.Title,
		Body:      t.Body,
		Author:    t.Author,
		Labels:    t.Labels,
		UpdatedAt: updatedAt,
	}
}

// Dossier builds a source-backed repository dossier from local corpus data.
func (r *MCPReader) Dossier(ctx context.Context, in mcpserver.RepoInput) (mcpserver.DossierOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.DossierOutput{}, err
	}
	if _, err := r.Service.openCorpus(ctx); err != nil {
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
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.SearchCodeOutput{}, err
	}
	matches, err := c.SearchCode(ctx, in.Query, ref, in.Limit)
	if err != nil {
		return mcpserver.SearchCodeOutput{}, fmt.Errorf("search code: %w", err)
	}
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
	return mcpserver.SearchCodeOutput{Query: in.Query, Total: len(out), Matches: out}, nil
}

// Investigation reads a local investigation workspace from the corpus.
func (r *MCPReader) Investigation(ctx context.Context, in mcpserver.InvestigationInput) (mcpserver.InvestigationOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return mcpserver.InvestigationOutput{}, errors.New("id is required")
	}
	c, err := r.Service.openCorpus(ctx)
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
		ID:         inv.ID,
		Owner:      inv.Repo.Owner,
		Repo:       inv.Repo.Repo,
		CommitSHA:  inv.CommitSHA,
		Lens:       inv.Lens,
		Status:     string(inv.Status),
		CreatedAt:  formatTime(inv.CreatedAt),
		UpdatedAt:  formatTime(inv.UpdatedAt),
		Hypotheses: hyps,
	}, nil
}

// ListOpportunities lists opportunities for a local investigation.
func (r *MCPReader) ListOpportunities(ctx context.Context, in mcpserver.ListOpportunitiesInput) (mcpserver.ListOpportunitiesOutput, error) {
	if strings.TrimSpace(in.InvestigationID) == "" {
		return mcpserver.ListOpportunitiesOutput{}, errors.New("investigation_id is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.ListOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.Service.openCorpus(ctx)
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
	if strings.TrimSpace(in.ID) == "" {
		return mcpserver.OpportunityOutput{}, errors.New("id is required")
	}
	c, err := r.Service.openCorpus(ctx)
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
		EvidenceIDs:         evidenceIDs,
		Status:              string(opp.Status),
		CreatedAt:           formatTime(opp.CreatedAt),
		UpdatedAt:           formatTime(opp.UpdatedAt),
	}, nil
}

// Evidence reads evidence for a local investigation or opportunity.
func (r *MCPReader) Evidence(ctx context.Context, in mcpserver.EvidenceInput) (mcpserver.EvidenceOutput, error) {
	if in.InvestigationID == "" && in.OpportunityID == "" {
		return mcpserver.EvidenceOutput{}, errors.New("investigation_id or opportunity_id is required")
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
	c, err := r.Service.openCorpus(ctx)
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
	for i, e := range items {
		out[i] = mcpserver.EvidenceItem{
			ID:          e.ID,
			Type:        string(e.Type),
			Relation:    string(e.Relation),
			Description: e.Description,
			SourceRefs:  sourceRefsToMCP(e.SourceRefs),
			CreatedAt:   formatTime(e.CreatedAt),
		}
	}
	return mcpserver.EvidenceOutput{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
		Total:           total,
		Evidence:        out,
	}, nil
}

func dossierToMCPOutput(d *domain.Dossier) mcpserver.DossierOutput {
	return mcpserver.DossierOutput{
		Owner: d.Repo.Owner,
		Repo:  d.Repo.Repo,
		AsOf:  d.AsOf.Format(time.RFC3339),
		Sections: map[string]any{
			"description":                d.Repository.Description,
			"language":                   firstLanguage(d.Repository.Languages),
			"stars":                      d.Repository.Stars,
			"open_issues":                d.OpenIssueCount,
			"closed_issues":              d.ClosedIssueCount,
			"open_prs":                   d.OpenPullRequestCount,
			"merged_prs":                 d.MergedPullRequestCount,
			"closed_unmerged_prs":        d.ClosedUnmergedPullRequestCount,
			"recent_merged_prs":          d.RecentMergedPullRequests,
			"recent_open_prs":            d.RecentOpenPullRequests,
			"recent_closed_unmerged_prs": d.RecentClosedUnmergedPullRequests,
			"recent_issues":              d.RecentIssues,
			"guidance":                   d.ContributionGuidance,
			"coverage":                   coverageNames(d.Coverage),
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func isValidEvidenceRelation(s string) bool {
	switch evidence.Relation(s) {
	case evidence.RelationSupporting, evidence.RelationContradicting, evidence.RelationInconclusive, evidence.RelationStale, evidence.RelationInvalid:
		return true
	}
	return false
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
	server := mcpserver.New(r.Service.MCPReader(), r.Service.version)
	return server.ServeStdio(ctx)
}
