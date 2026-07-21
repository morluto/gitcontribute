package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

const (
	readinessRuleSetVersion = "readiness.v1"
	readinessRuleVersion    = "v1"

	readinessPass    = "pass"
	readinessWarn    = "warn"
	readinessBlock   = "block"
	readinessUnknown = "unknown"
)

// OpportunityReadiness evaluates local, source-backed readiness checks for one opportunity.
func (s *Service) OpportunityReadiness(ctx context.Context, opportunityID string) (*cli.ReadinessResult, error) {
	if strings.TrimSpace(opportunityID) == "" {
		return nil, errors.New("opportunity id is required")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	invSvc := investigation.NewService(c, c)
	opp, err := invSvc.GetOpportunity(ctx, opportunityID)
	if err != nil {
		return nil, mapInvestigationError(err)
	}
	inv, err := invSvc.GetInvestigation(ctx, opp.InvestigationID)
	if err != nil {
		return nil, mapInvestigationError(err)
	}

	evaluator := &readinessEvaluator{
		ctx:         ctx,
		corpus:      c,
		service:     s,
		opportunity: opp,
		inv:         inv,
		evaluatedAt: s.now().UTC(),
	}
	checks, err := evaluator.evaluate()
	if err != nil {
		return nil, err
	}
	return &cli.ReadinessResult{
		OpportunityID:  opp.ID,
		RuleSetVersion: readinessRuleSetVersion,
		Status:         aggregateReadinessStatus(checks),
		EvaluatedAt:    formatTime(evaluator.evaluatedAt),
		Checks:         checks,
	}, nil
}

// ExplainReadiness re-evaluates readiness and returns the requested check.
func (s *Service) ExplainReadiness(ctx context.Context, checkID string) (*cli.ReadinessCheck, error) {
	opportunityID, ruleID, ok := strings.Cut(strings.TrimSpace(checkID), ":")
	if !ok || opportunityID == "" || ruleID == "" {
		return nil, fmt.Errorf("invalid readiness check id %q: expected <opportunity-id>:<rule-id>", checkID)
	}
	report, err := s.OpportunityReadiness(ctx, opportunityID)
	if err != nil {
		return nil, err
	}
	for i := range report.Checks {
		if report.Checks[i].CheckID == checkID || report.Checks[i].RuleID == ruleID {
			return &report.Checks[i], nil
		}
	}
	return nil, fmt.Errorf("readiness check %q not found", checkID)
}

type readinessEvaluator struct {
	ctx         context.Context
	corpus      *corpus.Corpus
	service     *Service
	opportunity *investigation.Opportunity
	inv         *investigation.Investigation
	evaluatedAt time.Time

	repository *corpus.Repository
	evidence   []*evidence.Evidence
	defs       []*evidence.ValidationDefinition
	runs       []*evidence.ValidationRun
}

func (r *readinessEvaluator) evaluate() ([]cli.ReadinessCheck, error) {
	if err := r.load(); err != nil {
		return nil, err
	}
	var checks []cli.ReadinessCheck
	add := func(check cli.ReadinessCheck, err error) error {
		if err != nil {
			return err
		}
		checks = append(checks, check)
		return nil
	}
	for _, fn := range []func() (cli.ReadinessCheck, error){
		r.repositoryArchived,
		r.targetThreadOpen,
		r.baselineFresh,
		r.guidancePresent,
		r.collisionStatus,
		r.validationPresence,
		r.candidateImprovesBaseline,
		r.evidenceFreshness,
		r.contradictingEvidence,
		r.draftReferencesEvidence,
	} {
		if err := add(fn()); err != nil {
			return nil, err
		}
	}
	return checks, nil
}

func (r *readinessEvaluator) load() error {
	repo, err := r.corpus.GetRepository(r.ctx, r.inv.Repo.Owner, r.inv.Repo.Repo)
	if err != nil {
		return fmt.Errorf("read readiness repository: %w", err)
	}
	r.repository = repo

	items, err := r.corpus.ListEvidence(r.ctx, evidence.EvidenceFilter{OpportunityID: r.opportunity.ID})
	if err != nil {
		return fmt.Errorf("read readiness evidence: %w", err)
	}
	r.evidence = items

	defs, err := r.corpus.ListValidationDefinitions(r.ctx, r.opportunity.ID)
	if err != nil {
		return fmt.Errorf("read readiness validation definitions: %w", err)
	}
	r.defs = defs

	runs, err := r.corpus.ListValidationRuns(r.ctx, r.opportunity.ID)
	if err != nil {
		return fmt.Errorf("read readiness validation runs: %w", err)
	}
	r.runs = runs
	return nil
}

func (r *readinessEvaluator) repositoryArchived() (cli.ReadinessCheck, error) {
	if r.repository == nil {
		return r.check("repository_archived", readinessUnknown, "Repository metadata is not present in the local corpus.", nil, "Run an explicit sync before preparing the contribution."), nil
	}
	if r.repository.Archived {
		return r.check("repository_archived", readinessBlock, "Repository is archived.", []string{"repo:" + r.inv.Repo.String()}, "Do not submit; choose an active repository or confirm maintainers accept contributions elsewhere."), nil
	}
	return r.check("repository_archived", readinessPass, "Repository is not archived.", []string{"repo:" + r.inv.Repo.String()}, ""), nil
}

func (r *readinessEvaluator) targetThreadOpen() (cli.ReadinessCheck, error) {
	if r.inv.ThreadBaseline == nil {
		return r.check("target_thread_open", readinessUnknown, "Opportunity is not tied to a stored target thread baseline.", nil, "Start from a stored issue or PR thread, or add an explicit source reference."), nil
	}
	if r.repository == nil {
		return r.check("target_thread_open", readinessUnknown, "Target thread cannot be checked because repository metadata is missing.", []string{r.inv.ThreadBaseline.Ref()}, "Run an explicit sync for the repository."), nil
	}
	thread, err := r.corpus.GetThread(r.ctx, r.repository.ID, string(r.inv.ThreadBaseline.Kind), r.inv.ThreadBaseline.Number)
	if err != nil {
		return cli.ReadinessCheck{}, fmt.Errorf("read readiness target thread: %w", err)
	}
	ref := r.inv.ThreadBaseline.Ref()
	if thread == nil {
		return r.check("target_thread_open", readinessUnknown, "Target thread is not present in the current local corpus projection.", []string{ref}, "Refresh the target thread before preparing the contribution."), nil
	}
	if thread.State != "open" {
		return r.check("target_thread_open", readinessBlock, fmt.Sprintf("Target thread is %s.", thread.State), []string{ref}, "Do not submit until the target is reopened or the contribution is retargeted."), nil
	}
	return r.check("target_thread_open", readinessPass, "Target thread is open.", []string{ref}, ""), nil
}

func (r *readinessEvaluator) baselineFresh() (cli.ReadinessCheck, error) {
	if r.inv.ThreadBaseline == nil {
		return r.check("baseline_freshness", readinessUnknown, "No immutable thread baseline is recorded.", nil, "Start from a stored thread or re-check the target manually."), nil
	}
	item := &evidence.Evidence{
		Type:             evidence.EvidenceTypeGitHubSource,
		SourceProvenance: []evidence.SourceRevision{sourceRevisionFromThreadBaseline(*r.inv.ThreadBaseline)},
	}
	freshness, err := evidence.NewFreshnessEvaluator(r.corpus).Evaluate(r.ctx, item)
	if err != nil {
		return cli.ReadinessCheck{}, fmt.Errorf("evaluate readiness baseline freshness: %w", err)
	}
	switch freshness.Status {
	case evidence.FreshnessFresh:
		return r.check("baseline_freshness", readinessPass, "Thread baseline matches the current local projection.", []string{r.inv.ThreadBaseline.Ref()}, ""), nil
	case evidence.FreshnessStale:
		return r.check("baseline_freshness", readinessWarn, "Thread baseline is stale: "+freshness.Reason, []string{r.inv.ThreadBaseline.Ref()}, "Re-read the thread research brief before preparing the contribution."), nil
	default:
		return r.check("baseline_freshness", readinessUnknown, "Thread baseline freshness is unknown: "+freshness.Reason, []string{r.inv.ThreadBaseline.Ref()}, "Refresh or inspect the target thread before relying on this baseline."), nil
	}
}

func (r *readinessEvaluator) guidancePresent() (cli.ReadinessCheck, error) {
	guidance, refs, err := (&corpusReader{s: r.service}).ReadContributionGuidance(r.ctx, r.inv.Repo)
	if err != nil {
		return cli.ReadinessCheck{}, fmt.Errorf("read readiness guidance: %w", err)
	}
	if strings.TrimSpace(guidance) == "" || len(refs) == 0 {
		return r.check("guidance_present", readinessUnknown, "No source-backed contribution or AI guidance is present in the local corpus.", nil, "Explicitly refresh repository guidance before preparing a public submission."), nil
	}
	return r.check("guidance_present", readinessPass, "Source-backed contribution guidance is available.", sourceRefStrings(refs), ""), nil
}

func (r *readinessEvaluator) collisionStatus() (cli.ReadinessCheck, error) {
	switch r.opportunity.CollisionStatus {
	case investigation.CollisionNone:
		return r.check("collision_status", readinessPass, "No known competing work is recorded.", nil, ""), nil
	case investigation.CollisionPossible:
		return r.check("collision_status", readinessWarn, "Possible competing work is recorded.", nil, "Review collision evidence before preparing the contribution."), nil
	case investigation.CollisionConfirmed, investigation.CollisionBlocked:
		return r.check("collision_status", readinessBlock, "Confirmed competing work blocks this opportunity.", nil, "Resolve or retarget the opportunity before preparing the contribution."), nil
	default:
		return r.check("collision_status", readinessUnknown, "Collision status is unknown.", nil, "Run duplicate/collision checks or record an explicit collision decision."), nil
	}
}

func (r *readinessEvaluator) validationPresence() (cli.ReadinessCheck, error) {
	refs := validationRefs(r.runs)
	switch {
	case len(r.defs) == 0:
		return r.check("validation_presence", readinessUnknown, "No validation definition is scoped to this opportunity.", nil, "Define at least one explicit validation before preparing a PR."), nil
	case len(r.runs) == 0:
		return r.check("validation_presence", readinessWarn, "Validation is defined but has no recorded runs.", validationDefinitionRefs(r.defs), "Run the validation before preparing the contribution."), nil
	default:
		return r.check("validation_presence", readinessPass, "Validation definitions and recorded runs are present.", refs, ""), nil
	}
}

func (r *readinessEvaluator) candidateImprovesBaseline() (cli.ReadinessCheck, error) {
	if len(r.runs) == 0 {
		return r.check("candidate_improves_baseline", readinessUnknown, "No validation runs are available to compare base and candidate behavior.", nil, "Run base and candidate validation before preparing a PR."), nil
	}
	baseFailing := map[string]*evidence.ValidationRun{}
	var candidateRefs []string
	var errorRefs []string
	for _, run := range r.runs {
		switch run.Kind {
		case evidence.RunKindBase:
			if run.Classification == evidence.RunClassificationFailing {
				baseFailing[run.DefinitionID] = run
			}
		case evidence.RunKindCandidate:
			candidateRefs = append(candidateRefs, "validation_run:"+run.ID)
			switch run.Classification {
			case evidence.RunClassificationFailing:
				return r.check("candidate_improves_baseline", readinessBlock, "Candidate validation is failing.", []string{"validation_run:" + run.ID}, "Fix the candidate or record newer passing validation."), nil
			case evidence.RunClassificationError, evidence.RunClassificationCancelled:
				errorRefs = append(errorRefs, "validation_run:"+run.ID)
			case evidence.RunClassificationPassing:
				if base := baseFailing[run.DefinitionID]; base != nil {
					comparison, err := evidence.Compare(base, run)
					if err != nil {
						return cli.ReadinessCheck{}, fmt.Errorf("compare readiness validation runs: %w", err)
					}
					refs := []string{"validation_run:" + base.ID, "validation_run:" + run.ID}
					if comparison.Classification == evidence.ComparisonFixed {
						return r.check("candidate_improves_baseline", readinessPass, "Candidate validation fixes a failing base run.", refs, ""), nil
					}
					if comparison.Classification == evidence.ComparisonInconclusive {
						return r.check("candidate_improves_baseline", readinessWarn, "Validation outcomes improved, but the expected observation contract did not match.", refs, "Rerun validation against the intended symptom or revise the observation contract."), nil
					}
				}
			}
		}
	}
	if len(errorRefs) > 0 {
		return r.check("candidate_improves_baseline", readinessWarn, "Candidate validation has error or cancelled runs.", errorRefs, "Rerun validation and record a clear candidate result."), nil
	}
	if len(candidateRefs) > 0 {
		return r.check("candidate_improves_baseline", readinessWarn, "Candidate validation exists, but no failing-base to passing-candidate pair is recorded.", candidateRefs, "Run base validation or record evidence explaining the improvement."), nil
	}
	return r.check("candidate_improves_baseline", readinessUnknown, "No candidate validation run is recorded.", validationDefinitionRefs(r.defs), "Run candidate validation before preparing a PR."), nil
}

func (r *readinessEvaluator) evidenceFreshness() (cli.ReadinessCheck, error) {
	if len(r.evidence) == 0 {
		return r.check("evidence_freshness", readinessUnknown, "No evidence is scoped to this opportunity.", nil, "Record supporting evidence before preparing the contribution."), nil
	}
	freshness := evidence.NewFreshnessEvaluator(r.corpus)
	var staleRefs, unknownRefs []string
	for _, item := range r.evidence {
		f, err := freshness.Evaluate(r.ctx, item)
		if err != nil {
			return cli.ReadinessCheck{}, fmt.Errorf("evaluate readiness evidence %q: %w", item.ID, err)
		}
		switch f.Status {
		case evidence.FreshnessStale:
			staleRefs = append(staleRefs, "evidence:"+item.ID)
		case evidence.FreshnessUnknown:
			unknownRefs = append(unknownRefs, "evidence:"+item.ID)
		}
	}
	if len(staleRefs) > 0 {
		return r.check("evidence_freshness", readinessWarn, "Some evidence is stale relative to the current local corpus.", staleRefs, "Re-check stale evidence before preparing the contribution."), nil
	}
	if len(unknownRefs) > 0 {
		return r.check("evidence_freshness", readinessUnknown, "Some evidence freshness is unknown.", unknownRefs, "Refresh source-backed evidence or confirm it manually."), nil
	}
	return r.check("evidence_freshness", readinessPass, "Evidence is fresh or local-only.", evidenceRefs(r.evidence), ""), nil
}

func (r *readinessEvaluator) contradictingEvidence() (cli.ReadinessCheck, error) {
	var refs []string
	for _, item := range r.evidence {
		if item.Relation == evidence.RelationContradicting {
			refs = append(refs, "evidence:"+item.ID)
		}
	}
	if len(refs) > 0 {
		return r.check("contradicting_evidence", readinessBlock, "Contradicting evidence is still scoped to this opportunity.", refs, "Resolve, invalidate, or explain the contradiction before preparing the contribution."), nil
	}
	return r.check("contradicting_evidence", readinessPass, "No contradicting evidence is scoped to this opportunity.", evidenceRefs(r.evidence), ""), nil
}

func (r *readinessEvaluator) draftReferencesEvidence() (cli.ReadinessCheck, error) {
	issue, issueErr := r.corpus.GetIssueDraft(r.ctx, r.opportunity.ID)
	if issueErr != nil && !errors.Is(issueErr, contribution.ErrNotFound) {
		return cli.ReadinessCheck{}, fmt.Errorf("read issue draft: %w", issueErr)
	}
	pr, prErr := r.corpus.GetPullRequestDraft(r.ctx, r.opportunity.ID)
	if prErr != nil && !errors.Is(prErr, contribution.ErrNotFound) {
		return cli.ReadinessCheck{}, fmt.Errorf("read pull request draft: %w", prErr)
	}
	var drafts []struct {
		kind string
		body string
	}
	if issue != nil {
		drafts = append(drafts, struct {
			kind string
			body string
		}{kind: "issue", body: issue.Body})
	}
	if pr != nil {
		drafts = append(drafts, struct {
			kind string
			body string
		}{kind: "pull_request", body: pr.Body})
	}
	if len(drafts) == 0 {
		return r.check("draft_references_evidence", readinessUnknown, "No local contribution draft is recorded.", nil, "Prepare an issue or pull request draft after resolving readiness warnings."), nil
	}
	if len(r.evidence) == 0 {
		return r.check("draft_references_evidence", readinessWarn, "A local draft exists but no opportunity evidence is recorded.", draftRefs(drafts), "Record evidence and regenerate the draft."), nil
	}
	for _, draft := range drafts {
		for _, item := range r.evidence {
			if evidenceMentionedInDraft(draft.body, item) {
				return r.check("draft_references_evidence", readinessPass, "A local draft references recorded evidence.", append(draftRefs(drafts), "evidence:"+item.ID), ""), nil
			}
		}
	}
	return r.check("draft_references_evidence", readinessWarn, "A local draft exists but does not appear to reference recorded evidence.", draftRefs(drafts), "Regenerate the draft after recording evidence."), nil
}

func (r *readinessEvaluator) check(ruleID, status, summary string, refs []string, remediation string) cli.ReadinessCheck {
	return cli.ReadinessCheck{
		CheckID:      r.opportunity.ID + ":" + ruleID,
		RuleID:       ruleID,
		RuleVersion:  readinessRuleVersion,
		Status:       status,
		Summary:      summary,
		EvidenceRefs: append([]string(nil), refs...),
		Remediation:  remediation,
		EvaluatedAt:  formatTime(r.evaluatedAt),
	}
}

func aggregateReadinessStatus(checks []cli.ReadinessCheck) string {
	status := readinessPass
	for _, check := range checks {
		switch check.Status {
		case readinessBlock:
			return readinessBlock
		case readinessWarn:
			status = readinessWarn
		case readinessUnknown:
			if status == readinessPass {
				status = readinessUnknown
			}
		}
	}
	return status
}

func sourceRefStrings(refs []domain.SourceRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.URL != "" {
			out = append(out, ref.URL)
		} else if ref.Source != "" {
			out = append(out, "source:"+ref.Source)
		}
	}
	return out
}

func validationDefinitionRefs(defs []*evidence.ValidationDefinition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, "validation_definition:"+def.ID)
	}
	return out
}

func validationRefs(runs []*evidence.ValidationRun) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, "validation_run:"+run.ID)
	}
	return out
}

func evidenceRefs(items []*evidence.Evidence) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "evidence:"+item.ID)
	}
	return out
}

func draftRefs(drafts []struct {
	kind string
	body string
}) []string {
	out := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		out = append(out, "draft:"+draft.kind)
	}
	return out
}

func evidenceMentionedInDraft(body string, item *evidence.Evidence) bool {
	if item == nil {
		return false
	}
	if item.ValidationRunID != "" && strings.Contains(body, item.ValidationRunID) {
		return true
	}
	description := strings.TrimSpace(item.Description)
	return description != "" && strings.Contains(body, description)
}
