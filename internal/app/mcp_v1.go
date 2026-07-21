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

// SearchRepositories performs a local-only repository search.
func (r *MCPReader) SearchRepositories(ctx context.Context, in mcpserver.SearchRepositoriesInput) (mcpserver.SearchRepositoriesOutput, error) {
	repoRef := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if in.Owner != "" || in.Repo != "" {
		if err := repoRef.Validate(); err != nil {
			return mcpserver.SearchRepositoriesOutput{}, err
		}
	}

	c, err := r.Service.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.SearchRepositoriesOutput{}, err
	}

	// Exact repository lookup when owner and repo are provided.
	if in.Owner != "" && in.Repo != "" {
		repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
		if err != nil {
			return mcpserver.SearchRepositoriesOutput{}, fmt.Errorf("get repository: %w", err)
		}
		if repo != nil {
			return mcpserver.SearchRepositoriesOutput{
				Query: in.Query,
				Total: 1,
				Matches: []mcpserver.RepositoryOutput{
					repositoryToMCPOutput(repo),
				},
			}, nil
		}
		return mcpserver.SearchRepositoriesOutput{Query: in.Query, Matches: []mcpserver.RepositoryOutput{}}, nil
	}

	res, err := r.Service.searchCorpus(ctx, in.Query, cli.SearchOptions{
		Kind:   "repos",
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
	if err != nil {
		return mcpserver.SearchRepositoriesOutput{}, err
	}

	matches := make([]mcpserver.RepositoryOutput, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = mcpserver.RepositoryOutput{
			Owner:     m.Repo.Owner,
			Repo:      m.Repo.Repo,
			UpdatedAt: formatTime(m.UpdatedAt),
			Fields:    m.Fields,
		}
	}
	return mcpserver.SearchRepositoriesOutput{Query: in.Query, Total: res.Total, Matches: matches, NextCursor: res.NextCursor}, nil
}

func repositoryToMCPOutput(repo *corpus.Repository) mcpserver.RepositoryOutput {
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
	}
}

// ThreadByNumber reads an issue or pull request by repository and number only.
func (r *MCPReader) ThreadByNumber(ctx context.Context, in mcpserver.ThreadByNumberInput) (mcpserver.ThreadOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.ThreadOutput{}, err
	}
	if in.Number < 1 {
		return mcpserver.ThreadOutput{}, errors.New("number must be positive")
	}
	c, err := r.Service.openReadOnlyCorpus(ctx)
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
	thread, err := c.GetThreadByNumber(ctx, repo.ID, in.Number)
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

// ExplainMatch explains why a search result matched.
func (r *MCPReader) ExplainMatch(ctx context.Context, in mcpserver.ExplainMatchInput) (mcpserver.ExplainMatchOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.ExplainMatchOutput{}, err
	}

	c, err := r.Service.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.ExplainMatchOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpserver.ExplainMatchOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
	}

	out := mcpserver.ExplainMatchOutput{
		Query: in.Query,
		Owner: in.Owner,
		Repo:  in.Repo,
		Kind:  in.Kind,
	}

	switch out.Kind {
	case "", "issue", "pull_request":
		if in.Number < 1 {
			return mcpserver.ExplainMatchOutput{}, errors.New("number is required for thread matches")
		}
		thread, err := c.GetThreadByNumber(ctx, repo.ID, in.Number)
		if err != nil {
			return mcpserver.ExplainMatchOutput{}, fmt.Errorf("get thread: %w", err)
		}
		if thread == nil {
			return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
		}
		if in.Kind != "" && thread.Kind != in.Kind {
			return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
		}
		out.Kind = thread.Kind
		out.Number = thread.Number
		out.Title = thread.Title
		out.Snippet = boundedText(thread.Body, 2000)
		out.State = thread.State
		fields := map[string]string{
			"title":  thread.Title,
			"body":   thread.Body,
			"author": thread.Author,
			"state":  thread.State,
		}
		if len(thread.Labels) > 0 {
			fields["labels"] = strings.Join(thread.Labels, " ")
		}
		sourceRevision := thread.SourceUpdatedAt
		if in.Query != "" {
			evidence, found, err := c.FindThreadSearchEvidence(ctx, thread.ID, in.Query)
			if err != nil {
				return mcpserver.ExplainMatchOutput{}, err
			}
			if found && evidence.Source != "thread" {
				fields[evidence.Source] = evidence.Text
				out.Snippet = boundedText(evidence.Excerpt, 2000)
				sourceRevision = evidence.SourceUpdatedAt
			}
		}
		out.MatchedFields, out.Score = matchTerms(in.Query, fields)
		out.SourceRevision = formatTime(sourceRevision)
		cov, _, err := readCoverageTarget(ctx, c, mcpserver.CoverageTarget{Owner: in.Owner, Repo: in.Repo})
		if err != nil {
			return mcpserver.ExplainMatchOutput{}, fmt.Errorf("read repository coverage: %w", err)
		}
		out.Facets = cov.Facets
		out.AsOf = cov.AsOf
	case "code":
		if in.Path != "" || in.Commit != "" {
			if in.Path == "" {
				return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
			}
			match, err := c.GetCodeDocument(ctx, ref, in.Path)
			if err != nil {
				return mcpserver.ExplainMatchOutput{}, fmt.Errorf("get code document: %w", err)
			}
			if match == nil {
				return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
			}
			if in.Commit != "" && match.Commit != in.Commit {
				return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
			}
			out.Kind = "code"
			out.Path = match.Path
			out.Commit = match.Commit
			out.Title = match.Path
			out.Snippet = boundedText(match.Content, 2000)
			out.SourceRevision = match.Commit
			out.AsOf = formatTime(match.SnapshotCreatedAt)
			if in.Query != "" {
				out.MatchedFields, out.Score = matchTerms(in.Query, map[string]string{"code": match.Content, "path": match.Path})
				if out.Score == 0 {
					return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
				}
			} else {
				out.Score = 1.0
			}
			break
		}

		matches, err := c.SearchCode(ctx, in.Query, ref, in.Limit)
		if err != nil {
			return mcpserver.ExplainMatchOutput{}, fmt.Errorf("search code: %w", err)
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
			return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
		}
		out.Kind = "code"
		out.Path = match.Path
		out.Commit = match.Commit
		out.Title = match.Path
		out.Snippet = boundedText(match.Content, 2000)
		out.MatchedFields, out.Score = matchTerms(in.Query, map[string]string{"code": match.Content, "path": match.Path})
		out.SourceRevision = match.Commit
		out.AsOf = formatTime(match.SnapshotCreatedAt)
	case "repo":
		out.Kind = "repo"
		out.Title = ref.String()
		out.Snippet = boundedText(repo.Description, 2000)
		fields := map[string]string{
			"name":        ref.String(),
			"description": repo.Description,
			"language":    repo.Language,
			"license":     repo.License,
			"topics":      strings.Join(repo.Topics, " "),
		}
		out.MatchedFields, out.Score = matchTerms(in.Query, fields)
		out.SourceRevision = formatTime(repo.SourceUpdatedAt)
		cov, _, err := readCoverageTarget(ctx, c, mcpserver.CoverageTarget{Owner: in.Owner, Repo: in.Repo})
		if err != nil {
			return mcpserver.ExplainMatchOutput{}, fmt.Errorf("read repository coverage: %w", err)
		}
		out.Facets = cov.Facets
		out.AsOf = cov.AsOf
	default:
		return mcpserver.ExplainMatchOutput{}, fmt.Errorf("unsupported match kind %q", in.Kind)
	}

	terms := queryTerms(in.Query)
	if len(terms) > 0 && out.Score == 0 {
		return mcpserver.ExplainMatchOutput{}, mcpserver.ErrNotFound
	}
	out.Reason = fmt.Sprintf("matched %d/%d terms in %s", int(out.Score*float64(len(terms))), len(terms), strings.Join(out.MatchedFields, ", "))
	if len(terms) == 0 {
		out.Score = 1.0
		out.Reason = "repository present in local corpus"
	}
	return out, nil
}

func queryTerms(query string) []string {
	terms := strings.Fields(strings.ToLower(query))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func matchTerms(query string, fields map[string]string) ([]string, float64) {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, 1.0
	}
	matchedFields := make([]string, 0, len(fields))
	allText := make([]string, 0, len(fields))
	for name, value := range fields {
		text := strings.ToLower(value)
		for _, term := range terms {
			if strings.Contains(text, term) {
				matchedFields = append(matchedFields, name)
				break
			}
		}
		allText = append(allText, text)
	}
	joined := strings.Join(allText, " ")
	found := 0
	for _, term := range terms {
		if strings.Contains(joined, term) {
			found++
		}
	}
	score := 0.0
	if len(terms) > 0 {
		score = float64(found) / float64(len(terms))
	}
	return matchedFields, score
}

// BuildRepositoryDossier submits a durable job that builds a repository dossier.
func (r *MCPReader) BuildRepositoryDossier(ctx context.Context, in mcpserver.BuildRepositoryDossierInput) (mcpserver.JobReference, error) {
	repo := cli.RepoRef{Owner: in.Owner, Repo: in.Repo}
	id, err := r.Service.submitJob(ctx, "build_repository_dossier", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("repository_dossier", jobProgressCounts(0, 1)); err != nil {
			return nil, err
		}
		res, err := r.Service.BuildRepositoryDossier(ctx, repo)
		if err != nil {
			return nil, err
		}
		if err := report("repository_dossier", jobProgressCounts(1, 1)); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return queuedJobReference(id, "build_repository_dossier", "dossier build job started"), nil
}

// CreateWorkspace submits a durable job that clones a remote and creates a worktree.
func (r *MCPReader) CreateWorkspace(ctx context.Context, in mcpserver.CreateWorkspaceInput) (mcpserver.JobReference, error) {
	opts := cli.WorkspaceCreateOptions{
		Remote:       in.Remote,
		BaseRef:      in.BaseRef,
		CandidateRef: in.CandidateRef,
		Name:         in.Name,
	}
	id, err := r.Service.submitJob(ctx, "create_workspace", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("workspace_creation", jobProgressCounts(0, 1)); err != nil {
			return nil, err
		}
		res, err := r.Service.CreateWorkspace(ctx, in.InvestigationID, opts)
		if err != nil {
			return nil, err
		}
		if err := report("workspace_creation", jobProgressCounts(1, 1)); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return queuedJobReference(id, "create_workspace", "workspace creation job started"), nil
}

// RunValidation submits a durable validation run.
func (r *MCPReader) RunValidation(ctx context.Context, in mcpserver.RunValidationInput) (mcpserver.JobReference, error) {
	runKind := evidence.RunKind(in.Kind)
	if runKind != evidence.RunKindBase && runKind != evidence.RunKindCandidate {
		return mcpserver.JobReference{}, errors.New("kind must be base or candidate")
	}
	if !in.Execute {
		return mcpserver.JobReference{}, errors.New("execute must be true to authorize host command execution")
	}
	opts := cli.RunValidationOptions{Kind: in.Kind, Execute: true}
	id, err := r.Service.submitJob(ctx, "run_validation", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("validation", jobProgressCounts(0, 1)); err != nil {
			return nil, err
		}
		res, err := r.Service.RunValidation(ctx, in.ID, opts)
		if err != nil {
			return nil, err
		}
		if err := report("validation", jobProgressCounts(1, 1)); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return queuedJobReference(id, "run_validation", "validation run started"), nil
}

// StartInvestigation creates a new investigation workspace.
func (r *MCPReader) StartInvestigation(ctx context.Context, in mcpserver.StartInvestigationInput) (mcpserver.InvestigationOutput, error) {
	res, err := r.Service.StartInvestigation(ctx, cli.RepoRef{Owner: in.Owner, Repo: in.Repo}, in.CommitSHA, in.Lens)
	if err != nil {
		return mcpserver.InvestigationOutput{}, err
	}
	return investigationResultToMCP(res), nil
}

func investigationResultToMCP(res *cli.InvestigationResult) mcpserver.InvestigationOutput {
	return mcpserver.InvestigationOutput{
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
func (r *MCPReader) RecordHypothesis(ctx context.Context, in mcpserver.RecordHypothesisInput) (mcpserver.HypothesisOutput, error) {
	sourceRefs, err := mcpSourceRefsToDomain(in.SourceRefs)
	if err != nil {
		return mcpserver.HypothesisOutput{}, err
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
	h, err := r.Service.CreateHypothesis(ctx, in.InvestigationID, input)
	if err != nil {
		return mcpserver.HypothesisOutput{}, err
	}
	return hypothesisToMCP(h), nil
}

func hypothesisToMCP(h *investigation.Hypothesis) mcpserver.HypothesisOutput {
	return mcpserver.HypothesisOutput{
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

func mcpSourceRefsToDomain(refs []mcpserver.SourceRef) ([]domain.SourceRef, error) {
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
func (r *MCPReader) CheckDuplicates(ctx context.Context, in mcpserver.CheckDuplicatesInput) (mcpserver.CheckOutput, error) {
	var result *DuplicateCheckResult
	var err error
	switch in.Target {
	case "hypothesis":
		result, err = r.Service.CheckHypothesisDuplicates(ctx, in.ID, in.Limit)
	case "opportunity":
		result, err = r.Service.CheckOpportunityDuplicates(ctx, in.ID, in.Limit)
	default:
		return mcpserver.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
	}
	if err != nil {
		return mcpserver.CheckOutput{}, err
	}
	return duplicateCheckResultToMCP(in.Target, in.ID, result), nil
}

// CheckCollisions finds open pull request collisions for a hypothesis or opportunity.
func (r *MCPReader) CheckCollisions(ctx context.Context, in mcpserver.CheckCollisionsInput) (mcpserver.CheckOutput, error) {
	var result *CollisionCheckResult
	var err error
	switch in.Target {
	case "hypothesis":
		result, err = r.Service.CheckHypothesisCollisions(ctx, in.ID, in.Limit)
	case "opportunity":
		result, err = r.Service.CheckOpportunityCollisions(ctx, in.ID, in.Limit)
	default:
		return mcpserver.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
	}
	if err != nil {
		return mcpserver.CheckOutput{}, err
	}
	return collisionCheckResultToMCP(in.Target, in.ID, result), nil
}

func duplicateCheckResultToMCP(target, id string, result *DuplicateCheckResult) mcpserver.CheckOutput {
	return mcpserver.CheckOutput{
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

func collisionCheckResultToMCP(target, id string, result *CollisionCheckResult) mcpserver.CheckOutput {
	findings := make([]evidence.Evidence, len(result.Findings))
	for i := range result.Findings {
		findings[i] = result.Findings[i]
	}
	return mcpserver.CheckOutput{
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

func evidenceToMCPItems(items []evidence.Evidence) []mcpserver.EvidenceItem {
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
	return out
}

// PromoteOpportunity promotes a hypothesis to an opportunity.
func (r *MCPReader) PromoteOpportunity(ctx context.Context, in mcpserver.PromoteOpportunityInput) (mcpserver.OpportunityOutput, error) {
	sourceRefs, err := mcpSourceRefsToDomain(in.SourceRefs)
	if err != nil {
		return mcpserver.OpportunityOutput{}, err
	}
	input := investigation.PromoteOpportunityInput{
		ProblemStatement:    in.ProblemStatement,
		Scope:               in.Scope,
		Impact:              in.Impact,
		ExpectedEffort:      in.ExpectedEffort,
		Confidence:          in.Confidence,
		Dependencies:        append([]string(nil), in.Dependencies...),
		MaintainerAlignment: in.MaintainerAlignment,
		SourceRefs:          sourceRefs,
	}
	o, err := r.Service.PromoteOpportunityWithInput(ctx, in.HypothesisID, input)
	if err != nil {
		return mcpserver.OpportunityOutput{}, err
	}
	return opportunityToMCP(o), nil
}

func opportunityToMCP(o *investigation.Opportunity) mcpserver.OpportunityOutput {
	evidenceTotal := len(o.EvidenceIDs)
	return mcpserver.OpportunityOutput{
		ID:                  o.ID,
		InvestigationID:     o.InvestigationID,
		HypothesisID:        o.HypothesisID,
		Title:               o.Title,
		ProblemStatement:    o.ProblemStatement,
		Category:            string(o.Category),
		Scope:               o.Scope,
		Impact:              o.Impact,
		Confidence:          o.Confidence,
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
func (r *MCPReader) DefineValidation(ctx context.Context, in mcpserver.DefineValidationInput) (mcpserver.ValidationOutput, error) {
	var timeout time.Duration
	if in.Timeout != "" {
		d, err := time.ParseDuration(in.Timeout)
		if err != nil {
			return mcpserver.ValidationOutput{}, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = d
	}
	opts := cli.DefineValidationOptions{
		Kind:           in.Kind,
		Command:        in.Command,
		WorkingDir:     in.WorkingDir,
		BaseWorkingDir: in.BaseWorkingDir,
		CandidateDir:   in.CandidateDir,
		Env:            append([]string(nil), in.Env...),
		Timeout:        timeout,
		MaxOutputBytes: in.MaxOutputBytes,
		Observation:    observationContractMCPToCLI(in.Observation),
	}
	res, err := r.Service.DefineValidation(ctx, in.InvestigationID, opts)
	if err != nil {
		return mcpserver.ValidationOutput{}, err
	}
	return validationResultToMCP(res), nil
}

func validationResultToMCP(res *cli.ValidationResult) mcpserver.ValidationOutput {
	return mcpserver.ValidationOutput{
		ID:              res.ID,
		InvestigationID: res.InvestigationID,
		Kind:            res.Kind,
		Command:         res.Command,
		WorkingDir:      res.WorkingDir,
		BaseWorkingDir:  res.BaseWorkingDir,
		CandidateDir:    res.CandidateDir,
		Env:             res.Env,
		Timeout:         res.Timeout,
		MaxOutputBytes:  res.MaxOutputBytes,
		Observation:     observationContractCLIToMCP(res.Observation),
		CreatedAt:       res.CreatedAt,
	}
}

func observationContractMCPToCLI(contract *mcpserver.ValidationObservationContract) *cli.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &cli.ValidationObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsMCPToCLI(contract.Observations, "base"),
		Candidate: expectedObservationsMCPToCLI(contract.Observations, "candidate"),
	}
}

func expectedObservationsMCPToCLI(items []mcpserver.ValidationExpectedObservation, run string) []cli.ValidationExpectedObservation {
	out := make([]cli.ValidationExpectedObservation, 0, len(items))
	for _, item := range items {
		if item.Run != run {
			continue
		}
		out = append(out, cli.ValidationExpectedObservation{
			Name: item.Name, Source: item.Source, Matcher: item.Matcher,
			Pattern: item.Pattern, Occurrence: item.Occurrence, Path: item.Path,
		})
	}
	return out
}

func observationContractCLIToMCP(contract *cli.ValidationObservationContract) *mcpserver.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &mcpserver.ValidationObservationContract{
		Intent:       contract.Intent,
		Observations: append(expectedObservationsCLIToMCP(contract.Base, "base"), expectedObservationsCLIToMCP(contract.Candidate, "candidate")...),
	}
}

func expectedObservationsCLIToMCP(items []cli.ValidationExpectedObservation, run string) []mcpserver.ValidationExpectedObservation {
	out := make([]mcpserver.ValidationExpectedObservation, len(items))
	for i, item := range items {
		out[i] = mcpserver.ValidationExpectedObservation{
			Run: run, Name: item.Name, Source: item.Source, Matcher: item.Matcher,
			Pattern: item.Pattern, Occurrence: item.Occurrence, Path: item.Path,
		}
	}
	return out
}

// PrepareContribution renders a contribution draft for an opportunity.
func (r *MCPReader) PrepareContribution(ctx context.Context, in mcpserver.PrepareContributionInput) (mcpserver.DraftOutput, error) {
	var draft *cli.DraftResult
	var err error
	switch in.Kind {
	case "issue":
		draft, err = r.Service.PrepareIssue(ctx, in.OpportunityID, cli.PrepareIssueOptions{
			Guidance: in.Guidance,
			Success:  in.Success,
		})
	case "pull_request":
		draft, err = r.Service.PreparePullRequest(ctx, in.OpportunityID, cli.PreparePROptions{
			WorkspaceID:   in.WorkspaceID,
			Approach:      in.Approach,
			Changes:       in.Changes,
			Compatibility: in.Compatibility,
			Limitations:   in.Limitations,
			LinkedIssue:   in.LinkedIssue,
			Guidance:      in.Guidance,
		})
	default:
		return mcpserver.DraftOutput{}, fmt.Errorf("unsupported contribution kind %q", in.Kind)
	}
	if err != nil {
		return mcpserver.DraftOutput{}, err
	}
	return draftResultToMCP(draft), nil
}

func draftResultToMCP(d *cli.DraftResult) mcpserver.DraftOutput {
	return mcpserver.DraftOutput{
		OpportunityID: d.OpportunityID,
		Kind:          d.Kind,
		Title:         d.Title,
		Body:          d.Body,
		RenderedAt:    d.RenderedAt,
	}
}
