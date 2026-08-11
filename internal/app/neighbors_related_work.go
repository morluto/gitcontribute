package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

type relatedWorkSubjectKind uint8

const (
	relatedWorkHypothesis relatedWorkSubjectKind = iota + 1
	relatedWorkOpportunity
)

func parseRelatedWorkSubjectKind(value string) (relatedWorkSubjectKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hypothesis":
		return relatedWorkHypothesis, nil
	case "opportunity":
		return relatedWorkOpportunity, nil
	default:
		return 0, fmt.Errorf("unknown related-work target %q", value)
	}
}

func (k relatedWorkSubjectKind) String() string {
	if k == relatedWorkOpportunity {
		return "opportunity"
	}
	return "hypothesis"
}

type relatedWorkSubject struct {
	kind          relatedWorkSubjectKind
	id            string
	investigation *investigation.Investigation
	query         clustering.Candidate
	hypothesisID  string
	opportunityID string
}

func (s *Service) loadRelatedWorkSubject(ctx context.Context, kind relatedWorkSubjectKind, id string) (relatedWorkSubject, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return relatedWorkSubject{}, err
	}
	id = strings.TrimSpace(id)
	var subject relatedWorkSubject
	subject.kind = kind
	switch kind {
	case relatedWorkHypothesis:
		hypothesis, err := invSvc.GetHypothesis(ctx, id)
		if err != nil {
			return relatedWorkSubject{}, mapInvestigationError(err)
		}
		subject.id = hypothesis.ID
		subject.hypothesisID = hypothesis.ID
		subject.investigation, err = invSvc.GetInvestigation(ctx, hypothesis.InvestigationID)
		if err != nil {
			return relatedWorkSubject{}, mapInvestigationError(err)
		}
		subject.query = candidateFromHypothesis(hypothesis, subject.investigation.Repo)
	case relatedWorkOpportunity:
		opportunity, err := invSvc.GetOpportunity(ctx, id)
		if err != nil {
			return relatedWorkSubject{}, mapInvestigationError(err)
		}
		subject.id = opportunity.ID
		subject.hypothesisID = opportunity.HypothesisID
		subject.opportunityID = opportunity.ID
		subject.investigation, err = invSvc.GetInvestigation(ctx, opportunity.InvestigationID)
		if err != nil {
			return relatedWorkSubject{}, mapInvestigationError(err)
		}
		subject.query = candidateFromOpportunity(opportunity, subject.investigation.Repo)
	default:
		return relatedWorkSubject{}, errors.New("related-work subject was not parsed")
	}
	return subject, nil
}

func (s *Service) duplicatesForRelatedWorkSubject(ctx context.Context, subject relatedWorkSubject, limit int) (*contracts.DuplicateCheckResult, error) {
	neighbors, revision, effectiveLimit, err := s.findSimilarThreads(ctx, subject.investigation.Repo, subject.query, allSimilarThreads, limit)
	if err != nil {
		return nil, err
	}
	findings := make([]evidence.Evidence, 0, len(neighbors))
	for _, n := range neighbors {
		findings = append(findings, evidenceFromNeighbor(n, subject.investigation.Repo, subject.investigation.ID, subject.hypothesisID, subject.opportunityID, evidence.RelationInconclusive))
	}
	hypothesisID := ""
	if subject.kind == relatedWorkHypothesis {
		hypothesisID = subject.hypothesisID
	}
	return &contracts.DuplicateCheckResult{
		HypothesisID:   hypothesisID,
		OpportunityID:  subject.opportunityID,
		Repo:           subject.investigation.Repo,
		Query:          subject.query.Title,
		Findings:       findings,
		SourceRevision: revision,
		Limit:          effectiveLimit,
		Total:          len(findings),
	}, nil
}

// CheckHypothesisDuplicates searches the local corpus for threads similar to
// a hypothesis, returning each finding as evidence.
func (s *Service) CheckHypothesisDuplicates(ctx context.Context, hypothesisID string, limit int) (*contracts.DuplicateCheckResult, error) {
	subject, err := s.loadRelatedWorkSubject(ctx, relatedWorkHypothesis, hypothesisID)
	if err != nil {
		return nil, err
	}
	return s.duplicatesForRelatedWorkSubject(ctx, subject, limit)
}

// CheckOpportunityDuplicates searches the local corpus for threads similar to
// an opportunity.
func (s *Service) CheckOpportunityDuplicates(ctx context.Context, opportunityID string, limit int) (*contracts.DuplicateCheckResult, error) {
	subject, err := s.loadRelatedWorkSubject(ctx, relatedWorkOpportunity, opportunityID)
	if err != nil {
		return nil, err
	}
	return s.duplicatesForRelatedWorkSubject(ctx, subject, limit)
}

// CheckHypothesisCollisions searches the local corpus for open pull requests
// that may collide with a hypothesis.
func (s *Service) CheckHypothesisCollisions(ctx context.Context, hypothesisID string, limit int) (*contracts.CollisionCheckResult, error) {
	subject, err := s.loadRelatedWorkSubject(ctx, relatedWorkHypothesis, hypothesisID)
	if err != nil {
		return nil, err
	}
	return s.collisionsForRelatedWorkSubject(ctx, subject, limit)
}

// CheckOpportunityCollisions searches the local corpus for open pull requests
// that may collide with an opportunity.
func (s *Service) CheckOpportunityCollisions(ctx context.Context, opportunityID string, limit int) (*contracts.CollisionCheckResult, error) {
	subject, err := s.loadRelatedWorkSubject(ctx, relatedWorkOpportunity, opportunityID)
	if err != nil {
		return nil, err
	}
	return s.collisionsForRelatedWorkSubject(ctx, subject, limit)
}

func (s *Service) collisionsForRelatedWorkSubject(ctx context.Context, subject relatedWorkSubject, limit int) (*contracts.CollisionCheckResult, error) {
	neighbors, revision, effectiveLimit, err := s.findSimilarThreads(ctx, subject.investigation.Repo, subject.query, openPullRequestsOnly, limit)
	if err != nil {
		return nil, err
	}
	findings := make([]evidence.Evidence, 0, len(neighbors))
	for _, n := range neighbors {
		findings = append(findings, evidenceFromNeighbor(n, subject.investigation.Repo, subject.investigation.ID, subject.hypothesisID, subject.opportunityID, evidence.RelationContradicting))
	}
	return &contracts.CollisionCheckResult{
		HypothesisID:   subject.hypothesisID,
		OpportunityID:  subject.opportunityID,
		Repo:           subject.investigation.Repo,
		Query:          subject.query.Title,
		Findings:       findings,
		SourceRevision: revision,
		Limit:          effectiveLimit,
		Total:          len(findings),
	}, nil
}

type similarThreadScope uint8

const (
	allSimilarThreads similarThreadScope = iota + 1
	openPullRequestsOnly
)

func (s similarThreadScope) filters() (corpus.ThreadKindFilter, corpus.ThreadStateFilter) {
	if s == openPullRequestsOnly {
		return corpus.PullRequestThreadKind(), corpus.OpenThreadState()
	}
	return corpus.AnyThreadKind(), corpus.AnyThreadState()
}

func (s *Service) findSimilarThreads(ctx context.Context, repo domain.RepoRef, query clustering.Candidate, scope similarThreadScope, limit int) ([]clustering.Neighbor, string, int, error) {
	if !repo.IsValid() {
		return nil, "", 0, errors.New("repository is required")
	}
	limit, err := normalizeSimilarityLimit(limit)
	if err != nil {
		return nil, "", 0, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, "", 0, err
	}
	repository, err := c.GetRepository(ctx, repo.Owner(), repo.Repo())
	if err != nil {
		return nil, "", 0, err
	}
	if repository == nil {
		// No local corpus data for this repository; return an empty result without
		// performing network access.
		return nil, "", limit, nil
	}
	kind, state := scope.filters()
	threads, err := c.ListThreadsFiltered(ctx, repository.ID, kind, state, similarityCandidateLimit(limit))
	if err != nil {
		return nil, "", 0, err
	}
	candidates := make([]clustering.Candidate, 0, len(threads))
	for _, t := range threads {
		candidates = append(candidates, candidateFromThread(repo, t))
	}
	all := append([]clustering.Candidate{query}, candidates...)
	neighbors, err := clustering.Neighbors(ctx, query, candidates, limit)
	if err != nil {
		return nil, "", 0, err
	}
	return neighbors, clustering.SourceRevision(all), limit, nil
}

func normalizeSimilarityLimit(limit int) (int, error) {
	if limit <= 0 {
		return defaultNeighborsLimit, nil
	}
	if limit > maxResultLimit {
		return 0, fmt.Errorf("neighbors limit cannot exceed %d", maxResultLimit)
	}
	return limit, nil
}

func similarityCandidateLimit(limit int) int {
	return min(maxCandidateLimit, max(minCandidateLimit, limit*candidateLimitFactor))
}

func candidateFromHypothesis(h *investigation.Hypothesis, repo domain.RepoRef) clustering.Candidate {
	body := h.Description
	if h.ExpectedBehavior != "" {
		body += "\n" + h.ExpectedBehavior
	}
	if h.ObservedBehavior != "" {
		body += "\n" + h.ObservedBehavior
	}
	if h.PotentialImpact != "" {
		body += "\n" + h.PotentialImpact
	}
	for _, ref := range h.SourceRefs {
		if ref.URL != "" {
			body += "\n" + ref.URL
		}
	}
	for _, link := range h.Links {
		if link.Ref != "" {
			body += "\n" + link.Ref
		}
	}
	return clustering.Candidate{Repo: repo, Title: h.Title, Body: body}
}

func candidateFromOpportunity(o *investigation.Opportunity, repo domain.RepoRef) clustering.Candidate {
	body := o.ProblemStatement
	if o.Scope != "" {
		body += "\n" + o.Scope
	}
	if o.Impact != "" {
		body += "\n" + o.Impact
	}
	for _, ref := range o.SourceRefs {
		if ref.URL != "" {
			body += "\n" + ref.URL
		}
	}
	return clustering.Candidate{Repo: repo, Title: o.Title, Body: body}
}

func evidenceFromNeighbor(n clustering.Neighbor, _ domain.RepoRef, investigationID, hypothesisID, opportunityID string, relation evidence.Relation) evidence.Evidence {
	path := "issues"
	if n.Ref.Kind == domain.PullRequestKind {
		path = "pull"
	}
	url := fmt.Sprintf("https://github.com/%s/%s/%s/%d", n.Ref.Owner, n.Ref.Repo, path, n.Ref.Number)
	now := time.Now().UTC()
	return evidence.Evidence{
		ID:              uuid.NewString(),
		InvestigationID: investigationID,
		HypothesisID:    hypothesisID,
		OpportunityID:   opportunityID,
		Type:            evidence.EvidenceTypeGitHubSource,
		Relation:        relation,
		Description:     fmt.Sprintf("possible related %s #%d: %s (score %.2f): %s", n.Ref.Kind, n.Ref.Number, n.Title, n.Score, n.Reason),
		SourceRefs:      []domain.SourceRef{{Source: "local-corpus", URL: url, ObservedAt: now}},
		CreatedAt:       now,
	}
}
