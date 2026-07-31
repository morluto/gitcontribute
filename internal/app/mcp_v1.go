package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/manifest"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/research"
)

// SearchRepositories performs a local-only repository search.
func (r *MCPReader) SearchRepositories(ctx context.Context, in mcpcontract.SearchRepositoriesInput) (mcpcontract.SearchRepositoriesOutput, error) {
	repoRef := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	repoFilter := ""
	if in.Owner != "" || in.Repo != "" {
		if err := repoRef.Validate(); err != nil {
			return mcpcontract.SearchRepositoriesOutput{}, err
		}
		repoFilter = repoRef.String()
	}

	res, err := r.searchCorpus(ctx, in.Query, contracts.SearchOptions{
		Kind:           "repos",
		Repo:           repoFilter,
		Limit:          in.Limit,
		Cursor:         in.Cursor,
		Sort:           in.Sort,
		CorpusRevision: in.CorpusRevision,
	})
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	if len(res.Matches) == 0 {
		provenance, err := offlineReadProvenance("repository_search", res.CorpusRevision, in, res.NextCursor == "", res.NextCursor != "", true)
		if err != nil {
			return mcpcontract.SearchRepositoriesOutput{}, err
		}
		return mcpcontract.SearchRepositoriesOutput{Query: in.Query, Total: res.Total, Matches: []mcpcontract.RepositoryOutput{}, NextCursor: res.NextCursor, CorpusRevision: res.CorpusRevision, Provenance: provenance}, nil
	}

	refs := make([]mcpcontract.RepositoryRef, len(res.Matches))
	for i, m := range res.Matches {
		refs[i] = mcpcontract.RepositoryRef{Owner: m.Repo.Owner, Repo: m.Repo.Repo}
	}
	batch, err := r.GetRepositories(ctx, mcpcontract.GetRepositoriesInput{Repositories: refs, CorpusRevision: &res.CorpusRevision})
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	matches := make([]mcpcontract.RepositoryOutput, 0, len(batch.Items))
	for _, item := range batch.Items {
		if item.Value != nil {
			matches = append(matches, *item.Value)
		}
	}
	provenance, err := offlineReadProvenance("repository_search", res.CorpusRevision, in, res.NextCursor == "", res.NextCursor != "", true)
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	return mcpcontract.SearchRepositoriesOutput{Query: in.Query, Total: res.Total, Matches: matches, NextCursor: res.NextCursor, CorpusRevision: res.CorpusRevision, Provenance: provenance}, nil
}

// ThreadByNumber reads an issue or pull request by repository and number only.
func (r *MCPReader) ThreadByNumber(ctx context.Context, in mcpcontract.ThreadByNumberInput) (mcpcontract.ThreadOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	if in.Number < 1 {
		return mcpcontract.ThreadOutput{}, errors.New("number must be positive")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, nil)
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
	thread, err := c.GetThreadByNumber(ctx, repo.ID, in.Number)
	if err != nil {
		return mcpcontract.ThreadOutput{}, fmt.Errorf("get thread: %w", err)
	}
	if thread == nil {
		return mcpcontract.ThreadOutput{}, failure.NotFound(nil)
	}
	out := corpusThreadToMCPOutput(thread)
	out.Owner = in.Owner
	out.Repo = in.Repo
	out.CorpusRevision = revision
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.ThreadOutput{}, err
	}
	return out, nil
}

// ExplainMatch explains why a search result matched.
func (r *MCPReader) ExplainMatch(ctx context.Context, in mcpcontract.ExplainMatchInput) (mcpcontract.ExplainMatchOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.ExplainMatchOutput{}, err
	}

	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ExplainMatchOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
	}

	out := mcpcontract.ExplainMatchOutput{
		Query: in.Query,
		Owner: in.Owner,
		Repo:  in.Repo,
		Kind:  in.Kind,
	}

	switch out.Kind {
	case "", "issue", "pull_request":
		if in.Number < 1 {
			return mcpcontract.ExplainMatchOutput{}, errors.New("number is required for thread matches")
		}
		thread, err := c.GetThreadByNumber(ctx, repo.ID, in.Number)
		if err != nil {
			return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("get thread: %w", err)
		}
		if thread == nil {
			return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
		}
		if in.Kind != "" && thread.Kind != in.Kind {
			return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
		}
		out.Kind = thread.Kind
		out.Number = thread.Number
		out.Title = thread.Title
		out.Snippet = boundedText(thread.Body, 2000)
		out.State = thread.State
		sourceRevision := thread.SourceUpdatedAt
		if in.Query != "" {
			evidence, found, err := c.FindThreadSearchEvidence(ctx, thread.ID, in.Query)
			if err != nil {
				return mcpcontract.ExplainMatchOutput{}, err
			}
			if !found {
				return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
			}
			rank := evidence.Rank
			out.RetrievalRank = &rank
			out.RankingMethod = "fts5_bm25_weighted"
			out.MatchSource = evidence.Source
			out.SearchTruncated = evidence.Truncated
			out.Snippet = boundedText(evidence.Excerpt, 2000)
			if evidence.Source != "thread" {
				sourceRevision = evidence.SourceUpdatedAt
			}
		}
		out.SourceRevision = formatTime(sourceRevision)
		cov, _, err := readCoverageTarget(ctx, c, mcpcontract.CoverageTarget{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: in.Owner, Repo: in.Repo}})
		if err != nil {
			return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("read repository coverage: %w", err)
		}
		out.Facets = cov.Facets
		out.AsOf = cov.AsOf
	case "code":
		if in.Path != "" || in.Commit != "" {
			if in.Path == "" {
				return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
			}
			match, err := c.GetCodeDocument(ctx, ref, in.Path)
			if err != nil {
				return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("get code document: %w", err)
			}
			if match == nil {
				return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
			}
			if in.Commit != "" && match.Commit != in.Commit {
				return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
			}
			out.Kind = "code"
			out.Path = match.Path
			out.Commit = match.Commit
			out.Title = match.Path
			out.Snippet = boundedText(match.Content, 2000)
			out.SourceRevision = match.Commit
			out.AsOf = formatTime(match.SnapshotCreatedAt)
			if in.Query != "" {
				if evidence, found, err := c.FindCodeSearchEvidence(ctx, match.DocID, in.Query); err != nil {
					return mcpcontract.ExplainMatchOutput{}, err
				} else if found {
					out.RetrievalRank, out.RankingMethod = &evidence.Rank, "fts5_bm25_weighted"
					out.Snippet = boundedText(evidence.Excerpt, 2000)
				} else {
					return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
				}
			}
			out.MatchSource = "code_document"
			break
		}

		matches, err := c.SearchCode(ctx, in.Query, ref, in.Limit)
		if err != nil {
			return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("search code: %w", err)
		}
		var match *corpus.CodeMatch
		for i := range matches {
			m := &matches[i]
			if (in.Path == "" || m.Path == in.Path) && (in.Commit == "" || m.Commit == in.Commit) {
				match = m
				break
			}
		}
		if match == nil && len(matches) > 0 {
			match = &matches[0]
		}
		if match == nil {
			return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
		}
		out.Kind = "code"
		out.Path = match.Path
		out.Commit = match.Commit
		out.Title = match.Path
		out.Snippet = boundedText(match.Content, 2000)
		out.MatchSource = "code_document"
		rank := match.Rank
		out.RetrievalRank, out.RankingMethod = &rank, "fts5_bm25_weighted"
		out.SourceRevision = match.Commit
		out.AsOf = formatTime(match.SnapshotCreatedAt)
	case "repo":
		out.Kind = "repo"
		out.Title = ref.String()
		out.Snippet = boundedText(repo.Description, 2000)
		if evidence, found, err := c.FindRepositorySearchEvidence(ctx, repo.ID, in.Query); err != nil {
			return mcpcontract.ExplainMatchOutput{}, err
		} else if found {
			out.RetrievalRank, out.RankingMethod = &evidence.Rank, "fts5_bm25_weighted"
			out.Snippet = boundedText(evidence.Excerpt, 2000)
			out.MatchSource = "repository_metadata"
		} else if in.Query != "" {
			return mcpcontract.ExplainMatchOutput{}, failure.NotFound(nil)
		}
		out.SourceRevision = formatTime(repo.SourceUpdatedAt)
		cov, _, err := readCoverageTarget(ctx, c, mcpcontract.CoverageTarget{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: in.Owner, Repo: in.Repo}})
		if err != nil {
			return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("read repository coverage: %w", err)
		}
		out.Facets = cov.Facets
		out.AsOf = cov.AsOf
	default:
		return mcpcontract.ExplainMatchOutput{}, fmt.Errorf("unsupported match kind %q", in.Kind)
	}

	if strings.TrimSpace(in.Query) == "" {
		out.Reason = "repository present in local corpus"
	} else {
		out.Reason = "matched by the stored weighted FTS5 document; retrieval_rank is the actual lower-is-better BM25 value"
	}
	return out, nil
}

// BuildRepositoryDossier submits a durable job that builds a repository dossier.
func (r *MCPReader) BuildRepositoryDossier(ctx context.Context, in mcpcontract.BuildRepositoryDossierInput) (mcpcontract.JobReference, error) {
	repo := contracts.RepoRef{Owner: in.Owner, Repo: in.Repo}
	id, err := r.submitJob(ctx, "build_repository_dossier", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("repository_dossier", jobProgressCounts(0, 1)); err != nil {
			return nil, err
		}
		res, err := r.application().BuildRepositoryDossier(ctx, repo)
		if err != nil {
			return nil, err
		}
		if err := report("repository_dossier", jobProgressCounts(1, 1)); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "build_repository_dossier", "dossier build job started"), nil
}

// CreateWorkspace submits a durable job that clones a remote and creates a worktree.
func (r *MCPReader) CreateWorkspace(ctx context.Context, in mcpcontract.CreateWorkspaceInput) (mcpcontract.JobReference, error) {
	opts := contracts.WorkspaceCreateOptions{
		Remote:       in.Remote,
		BaseRef:      in.BaseRef,
		CandidateRef: in.CandidateRef,
		Name:         in.Name,
	}
	id, err := r.submitJob(ctx, "create_workspace", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("workspace_creation", jobProgressCounts(0, 1)); err != nil {
			return nil, err
		}
		res, err := r.application().CreateWorkspace(ctx, in.InvestigationID, opts)
		if err != nil {
			return nil, err
		}
		if err := report("workspace_creation", jobProgressCounts(1, 1)); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "create_workspace", "workspace creation job started"), nil
}

// AdoptWorkspace records an existing worktree synchronously without exposing
// its host path or remote URL in the protocol result.
func (r *MCPReader) AdoptWorkspace(ctx context.Context, in mcpcontract.AdoptWorkspaceInput) (mcpcontract.AdoptWorkspaceOutput, error) {
	res, err := r.application().AdoptWorkspace(ctx, in.InvestigationID, contracts.WorkspaceAdoptOptions{Path: in.Path, BaseRef: in.BaseRef, Name: in.Name})
	if err != nil {
		return mcpcontract.AdoptWorkspaceOutput{}, err
	}
	return mcpcontract.AdoptWorkspaceOutput{
		ID: res.ID, InvestigationID: res.InvestigationID, Owner: res.Repo.Owner, Repo: res.Repo.Repo,
		BaseSHA: res.BaseSHA, CandidateSHA: res.CandidateSHA, MergeBase: res.MergeBase,
		Dirty: res.Dirty, HasUntracked: res.HasUntracked, Ownership: res.Ownership,
	}, nil
}

// RunValidation submits a durable validation run.
// StartInvestigation creates a new investigation workspace.
func (r *MCPReader) StartInvestigation(ctx context.Context, in mcpcontract.StartInvestigationInput) (mcpcontract.InvestigationOutput, error) {
	if in.Number > 0 {
		res, err := r.StartInvestigationFromThread(ctx, research.ThreadRef{
			Repo: domain.RepoRef{Owner: in.Owner, Repo: in.Repo}, Kind: domain.ThreadKind(in.Kind), Number: in.Number,
		})
		if err != nil {
			return mcpcontract.InvestigationOutput{}, err
		}
		out := investigationResultToMCP(res.Investigation)
		out.HypothesisTotal = 1
		out.Hypotheses = []mcpcontract.HypothesisSummary{{ID: res.Hypothesis.ID, Title: res.Hypothesis.Title, Category: res.Hypothesis.Category}}
		return out, nil
	}
	res, err := r.application().StartInvestigation(ctx, contracts.RepoRef{Owner: in.Owner, Repo: in.Repo}, in.CommitSHA, in.Lens)
	if err != nil {
		return mcpcontract.InvestigationOutput{}, err
	}
	return investigationResultToMCP(res), nil
}

func investigationResultToMCP(res *contracts.InvestigationResult) mcpcontract.InvestigationOutput {
	return mcpcontract.InvestigationOutput{
		ID:              res.ID,
		Owner:           res.Repo.Owner,
		Repo:            res.Repo.Repo,
		CommitSHA:       res.CommitSHA,
		Lens:            res.Lens,
		Status:          res.Status,
		CreatedAt:       res.CreatedAt,
		UpdatedAt:       res.UpdatedAt,
		HypothesisTotal: 0,
	}
}

// RecordHypothesis records a fully structured hypothesis.
func (r *MCPReader) RecordHypothesis(ctx context.Context, in mcpcontract.RecordHypothesisInput) (mcpcontract.HypothesisOutput, error) {
	sourceRefs, err := mcpSourceRefsToDomain(in.SourceRefs)
	if err != nil {
		return mcpcontract.HypothesisOutput{}, err
	}
	input := investigation.CreateHypothesisInput{
		Title:              in.Title,
		Description:        in.Description,
		Category:           investigation.Category(in.Category),
		ExpectedBehavior:   in.ExpectedBehavior,
		ObservedBehavior:   in.ObservedBehavior,
		PotentialImpact:    in.PotentialImpact,
		OpenQuestions:      append([]string(nil), in.OpenQuestions...),
		AffectedComponents: append([]string(nil), in.AffectedComponents...),
		SourceRefs:         sourceRefs,
	}
	h, err := r.CreateHypothesis(ctx, in.InvestigationID, input)
	if err != nil {
		return mcpcontract.HypothesisOutput{}, err
	}
	return hypothesisToMCP(h), nil
}

func hypothesisToMCP(h *investigation.Hypothesis) mcpcontract.HypothesisOutput {
	return mcpcontract.HypothesisOutput{
		ID:                 h.ID,
		InvestigationID:    h.InvestigationID,
		Title:              h.Title,
		Description:        h.Description,
		Category:           string(h.Category),
		ExpectedBehavior:   h.ExpectedBehavior,
		ObservedBehavior:   h.ObservedBehavior,
		PotentialImpact:    h.PotentialImpact,
		OpenQuestions:      h.OpenQuestions,
		AffectedComponents: h.AffectedComponents,
		SourceRefs:         sourceRefsToMCP(h.SourceRefs),
		Status:             string(h.Status),
		CreatedAt:          formatTime(h.CreatedAt),
		UpdatedAt:          formatTime(h.UpdatedAt),
	}
}

func mcpSourceRefsToDomain(refs []mcpcontract.SourceRef) ([]domain.SourceRef, error) {
	out := make([]domain.SourceRef, len(refs))
	for i, r := range refs {
		observedAt, err := parseTime(r.ObservedAt)
		if err != nil {
			return nil, fmt.Errorf("source_refs[%d].observed_at: %w", i, err)
		}
		asOf, err := parseTime(r.AsOf)
		if err != nil {
			return nil, fmt.Errorf("source_refs[%d].as_of: %w", i, err)
		}
		out[i] = domain.SourceRef{
			Source:     r.Source,
			URL:        r.URL,
			CommitSHA:  r.CommitSHA,
			ObservedAt: observedAt,
			AsOf:       asOf,
		}
	}
	return out, nil
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// CheckDuplicates finds duplicate-candidate threads for a hypothesis or opportunity.
func (r *MCPReader) CheckDuplicates(ctx context.Context, in mcpcontract.CheckDuplicatesInput) (mcpcontract.CheckOutput, error) {
	var result *contracts.DuplicateCheckResult
	var err error
	switch in.Target {
	case "hypothesis":
		result, err = r.CheckHypothesisDuplicates(ctx, in.ID, in.Limit)
	case "opportunity":
		result, err = r.CheckOpportunityDuplicates(ctx, in.ID, in.Limit)
	default:
		return mcpcontract.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
	}
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	return duplicateCheckResultToMCP(in.Target, in.ID, result), nil
}

// CheckCollisions finds open pull request collisions for a hypothesis or opportunity.
func (r *MCPReader) CheckCollisions(ctx context.Context, in mcpcontract.CheckCollisionsInput) (mcpcontract.CheckOutput, error) {
	var result *contracts.CollisionCheckResult
	var err error
	switch in.Target {
	case "hypothesis":
		result, err = r.CheckHypothesisCollisions(ctx, in.ID, in.Limit)
	case "opportunity":
		result, err = r.CheckOpportunityCollisions(ctx, in.ID, in.Limit)
	default:
		return mcpcontract.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
	}
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	return collisionCheckResultToMCP(in.Target, in.ID, result), nil
}

func duplicateCheckResultToMCP(target, id string, result *contracts.DuplicateCheckResult) mcpcontract.CheckOutput {
	return mcpcontract.CheckOutput{
		Target:         target,
		ID:             id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(result.Findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func collisionCheckResultToMCP(target, id string, result *contracts.CollisionCheckResult) mcpcontract.CheckOutput {
	findings := make([]evidence.Evidence, len(result.Findings))
	copy(findings, result.Findings)
	return mcpcontract.CheckOutput{
		Target:         target,
		ID:             id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func evidenceToMCPItems(items []evidence.Evidence) []mcpcontract.EvidenceItem {
	out := make([]mcpcontract.EvidenceItem, len(items))
	for i, e := range items {
		out[i] = mcpcontract.EvidenceItem{
			ID:          e.ID,
			Type:        string(e.Type),
			Relation:    string(e.Relation),
			Description: e.Description,
			SourceRefs:  sourceRefsToMCP(e.SourceRefs),
			CreatedAt:   formatTime(e.CreatedAt),
		}
	}
	return out
}

// PromoteOpportunity promotes a hypothesis to an opportunity.
func (r *MCPReader) PromoteOpportunity(ctx context.Context, in mcpcontract.PromoteOpportunityInput) (mcpcontract.OpportunityOutput, error) {
	sourceRefs, err := mcpSourceRefsToDomain(in.SourceRefs)
	if err != nil {
		return mcpcontract.OpportunityOutput{}, err
	}
	input := investigation.PromoteOpportunityInput{
		ProblemStatement:    in.ProblemStatement,
		Scope:               in.Scope,
		Impact:              in.Impact,
		ExpectedEffort:      in.ExpectedEffort,
		Confidence:          float64(in.Confidence),
		Dependencies:        append([]string(nil), in.Dependencies...),
		MaintainerAlignment: in.MaintainerAlignment,
		SourceRefs:          sourceRefs,
	}
	o, err := r.PromoteOpportunityWithInput(ctx, in.HypothesisID, input)
	if err != nil {
		return mcpcontract.OpportunityOutput{}, err
	}
	return opportunityToMCP(o), nil
}

func opportunityToMCP(o *investigation.Opportunity) mcpcontract.OpportunityOutput {
	evidenceTotal := len(o.EvidenceIDs)
	return mcpcontract.OpportunityOutput{
		ID:                  o.ID,
		InvestigationID:     o.InvestigationID,
		HypothesisID:        o.HypothesisID,
		Title:               o.Title,
		ProblemStatement:    o.ProblemStatement,
		Category:            string(o.Category),
		Scope:               o.Scope,
		Impact:              o.Impact,
		Confidence:          mcpcontract.Probability(o.Confidence),
		ExpectedEffort:      o.ExpectedEffort,
		Dependencies:        o.Dependencies,
		CollisionStatus:     string(o.CollisionStatus),
		MaintainerAlignment: o.MaintainerAlignment,
		SourceRefs:          sourceRefsToMCP(o.SourceRefs),
		EvidenceTotal:       evidenceTotal,
		EvidenceIDs:         append([]string(nil), o.EvidenceIDs...),
		Status:              string(o.Status),
		CreatedAt:           formatTime(o.CreatedAt),
		UpdatedAt:           formatTime(o.UpdatedAt),
	}
}

// DefineValidation stores a validation definition.
func (r *MCPReader) DefineValidation(ctx context.Context, in mcpcontract.DefineValidationInput) (mcpcontract.ValidationOutput, error) {
	var timeout time.Duration
	if in.Timeout != "" {
		d, err := time.ParseDuration(in.Timeout)
		if err != nil {
			return mcpcontract.ValidationOutput{}, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = d
	}
	var readinessTimeout time.Duration
	if in.ReadinessTimeout != "" {
		d, err := time.ParseDuration(in.ReadinessTimeout)
		if err != nil {
			return mcpcontract.ValidationOutput{}, fmt.Errorf("invalid readiness timeout: %w", err)
		}
		readinessTimeout = d
	}
	opts := contracts.DefineValidationOptions{
		Kind:                 in.Kind,
		Command:              in.Command,
		WorkspaceID:          in.WorkspaceID,
		BaseWorkspaceID:      in.BaseWorkspaceID,
		CandidateWorkspaceID: in.CandidateWorkspaceID,
		Env:                  append([]string(nil), in.Env...),
		Timeout:              timeout,
		MaxOutputBytes:       in.MaxOutputBytes,
		Observation:          observationContractMCPToCLI(in.Observation),
		Protocol:             in.Protocol,
		ReadinessTimeout:     readinessTimeout,
	}
	res, err := r.application().DefineValidation(ctx, in.InvestigationID, opts)
	if err != nil {
		return mcpcontract.ValidationOutput{}, err
	}
	return validationResultToMCP(res), nil
}

func validationResultToMCP(res *contracts.ValidationResult) mcpcontract.ValidationOutput {
	return mcpcontract.ValidationOutput{
		ID:                   res.ID,
		InvestigationID:      res.InvestigationID,
		Kind:                 res.Kind,
		Command:              res.Command,
		WorkingDir:           res.WorkingDir,
		BaseWorkingDir:       res.BaseWorkingDir,
		CandidateDir:         res.CandidateDir,
		WorkspaceID:          res.WorkspaceID,
		BaseWorkspaceID:      res.BaseWorkspaceID,
		CandidateWorkspaceID: res.CandidateWorkspaceID,
		Env:                  res.Env,
		Timeout:              res.Timeout,
		MaxOutputBytes:       res.MaxOutputBytes,
		Observation:          observationContractCLIToMCP(res.Observation),
		Protocol:             res.Protocol,
		ReadinessTimeout:     res.ReadinessTimeout,
		CreatedAt:            res.CreatedAt,
	}
}

func observationContractMCPToCLI(contract *mcpcontract.ValidationObservationContract) *contracts.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &contracts.ValidationObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsMCPToCLI(contract.Observations, "base"),
		Candidate: expectedObservationsMCPToCLI(contract.Observations, "candidate"),
	}
}

func expectedObservationsMCPToCLI(items []mcpcontract.ValidationExpectedObservation, run string) []contracts.ValidationExpectedObservation {
	out := make([]contracts.ValidationExpectedObservation, 0, len(items))
	for _, item := range items {
		if item.Run != run {
			continue
		}
		out = append(out, contracts.ValidationExpectedObservation{
			Name: item.Name, Source: item.Source, Matcher: item.Matcher,
			Pattern: item.Pattern, Occurrence: item.Occurrence, Path: item.Path,
		})
	}
	return out
}

func observationContractCLIToMCP(contract *contracts.ValidationObservationContract) *mcpcontract.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &mcpcontract.ValidationObservationContract{
		Intent:       contract.Intent,
		Observations: append(expectedObservationsCLIToMCP(contract.Base, "base"), expectedObservationsCLIToMCP(contract.Candidate, "candidate")...),
	}
}

func expectedObservationsCLIToMCP(items []contracts.ValidationExpectedObservation, run string) []mcpcontract.ValidationExpectedObservation {
	out := make([]mcpcontract.ValidationExpectedObservation, len(items))
	for i, item := range items {
		out[i] = mcpcontract.ValidationExpectedObservation{
			Run: run, Name: item.Name, Source: item.Source, Matcher: item.Matcher,
			Pattern: item.Pattern, Occurrence: item.Occurrence, Path: item.Path,
		}
	}
	return out
}

// PrepareContribution renders a contribution draft for an opportunity.
func (r *MCPReader) PrepareContribution(ctx context.Context, in mcpcontract.PrepareContributionInput) (mcpcontract.DraftOutput, error) {
	var draft *contracts.DraftResult
	var err error
	switch in.Kind {
	case "issue":
		draft, err = r.PrepareIssue(ctx, in.OpportunityID, contracts.PrepareIssueOptions{
			Guidance:   in.Guidance,
			Success:    in.Success,
			ManifestID: in.ManifestID,
		})
	case "pull_request":
		draft, err = r.PreparePullRequest(ctx, in.OpportunityID, contracts.PreparePROptions{
			WorkspaceID:   in.WorkspaceID,
			Approach:      in.Approach,
			Changes:       in.Changes,
			Compatibility: in.Compatibility,
			Limitations:   in.Limitations,
			LinkedIssue:   in.LinkedIssue,
			Guidance:      in.Guidance,
			ManifestID:    in.ManifestID,
		})
	default:
		return mcpcontract.DraftOutput{}, fmt.Errorf("unsupported contribution kind %q", in.Kind)
	}
	if err != nil {
		return mcpcontract.DraftOutput{}, err
	}
	return draftResultToMCP(draft), nil
}

func draftResultToMCP(d *contracts.DraftResult) mcpcontract.DraftOutput {
	out := mcpcontract.DraftOutput{
		ID:            d.ID,
		Revision:      d.Revision,
		OpportunityID: d.OpportunityID,
		Kind:          d.Kind,
		Repository:    d.Repository,
		Title:         d.Title,
		Body:          d.Body,
		TitleBytes:    d.TitleBytes,
		BodyBytes:     d.BodyBytes,
		TitleSHA256:   d.TitleSHA256,
		BodySHA256:    d.BodySHA256,
		EvidenceIDs:   append([]string(nil), d.EvidenceIDs...),
		RenderedAt:    d.RenderedAt,
		ManifestID:    d.ManifestID,
	}
	for _, warning := range d.Warnings {
		out.Warnings = append(out.Warnings, mcpcontract.DraftDiagnosticOutput{
			Code: warning.Code, Severity: warning.Severity, Message: warning.Message, ByteOffset: warning.ByteOffset,
		})
	}
	return out
}

func draftArtifactToMCP(d *contribution.DraftArtifact) mcpcontract.DraftOutput {
	out := mcpcontract.DraftOutput{
		ID: d.ID, Revision: d.Revision, OpportunityID: d.OpportunityID, Kind: d.Kind,
		Repository: d.Repository, Title: d.Title, Body: d.Body,
		TitleBytes: d.TitleBytes, BodyBytes: d.BodyBytes, TitleSHA256: d.TitleSHA256, BodySHA256: d.BodySHA256,
		EvidenceIDs: append([]string(nil), d.EvidenceIDs...), RenderedAt: d.RenderedAt.UTC().Format(time.RFC3339Nano),
		ManifestID: d.ManifestID,
	}
	for _, warning := range d.Warnings {
		out.Warnings = append(out.Warnings, mcpcontract.DraftDiagnosticOutput{
			Code: warning.Code, Severity: warning.Severity, Message: warning.Message, ByteOffset: warning.ByteOffset,
		})
	}
	return out
}

// ExportManifest assembles a bounded local contribution evidence statement.
func (r *MCPReader) ExportManifest(ctx context.Context, in mcpcontract.ExportManifestInput) (mcpcontract.ManifestOutput, error) {
	if in.CorpusRevision != nil && *in.CorpusRevision < 0 {
		return mcpcontract.ManifestOutput{}, mcpcontract.InvalidArgument("corpus_revision", "must be non-negative", map[string]any{"corpus_revision": 0})
	}
	opts := ManifestOptions{WorkspaceID: strings.TrimSpace(in.WorkspaceID)}
	if in.PullRequest != nil {
		opts.PullRequest = &ManifestPullRequest{Owner: strings.TrimSpace(in.PullRequest.Owner), Repo: strings.TrimSpace(in.PullRequest.Repo), Number: in.PullRequest.Number}
	}
	opts.CorpusRevision = in.CorpusRevision
	statement, revision, err := r.contributionManifestWithRevision(ctx, in.OpportunityID, opts)
	if err != nil {
		var stale *corpus.StaleCorpusRevisionError
		if errors.As(err, &stale) {
			return mcpcontract.ManifestOutput{}, mcpcontract.Unavailable(
				"corpus_revision_stale",
				fmt.Sprintf("requested corpus revision %d is no longer current; current revision is %d; reread after an explicit sync", stale.Expected, stale.Current),
			)
		}
		return mcpcontract.ManifestOutput{}, err
	}
	return manifestStatementToMCP(statement, revision), nil
}

func manifestStatementToMCP(statement *manifest.Statement, revision int64) mcpcontract.ManifestOutput {
	return mcpcontract.ManifestOutput{
		ManifestID: statement.Predicate.ManifestID, ContentSHA256: statement.Predicate.ContentSHA256,
		SchemaVersion: statement.Predicate.SchemaVersion, Status: statement.Predicate.Status, CorpusRevision: revision, Statement: *statement,
	}
}
