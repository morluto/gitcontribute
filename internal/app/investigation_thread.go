package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/research"
)

const maxThreadSeedDescription = 5000

// StartInvestigationFromThread atomically creates an investigation and seed
// hypothesis from one exact local observation. It performs no network access
// or process execution.
func (s *Service) StartInvestigationFromThread(ctx context.Context, requested research.ThreadRef) (*cli.ThreadInvestigationResult, error) {
	if err := requested.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	repo, err := c.GetRepository(ctx, requested.Repo.Owner, requested.Repo.Repo)
	if err != nil {
		return nil, fmt.Errorf("get thread investigation repository: %w", err)
	}
	if repo == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("%w: %s", errRepositoryNotFound, requested.Repo))
	}
	thread, err := c.GetThreadByNumber(ctx, repo.ID, requested.Number)
	if err != nil {
		return nil, fmt.Errorf("get thread investigation source: %w", err)
	}
	if thread == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("%w: %s#%d", research.ErrThreadNotFound, requested.Repo, requested.Number))
	}
	storedKind := domain.ThreadKind(thread.Kind)
	if requested.Kind != "" && requested.Kind != storedKind {
		return nil, cli.NewCLIError(cli.ExitNotFound, research.KindMismatchError(requested.Kind, storedKind))
	}
	resolved := research.ThreadRef{Repo: requested.Repo, Kind: storedKind, Number: requested.Number}
	observation, err := c.GetThreadObservationRevision(ctx, thread.ID, thread.SourceUpdatedAt, thread.ObservationSequence)
	if err != nil {
		if errors.Is(err, corpus.ErrThreadObservationRevisionNotFound) {
			return nil, fmt.Errorf("%w: projection %s has no matching observation revision", investigation.ErrInvalidThreadBaseline, resolved)
		}
		return nil, fmt.Errorf("read thread investigation baseline: %w", err)
	}
	description, truncated := boundedThreadSeedDescription(thread.Body)
	source := domain.SourceRef{
		Source: "github:rest", URL: fmt.Sprintf("https://api.github.com/repos/%s/issues/%d", resolved.Repo, resolved.Number),
		ObservedAt: observation.ObservedAt, AsOf: observation.SourceUpdatedAt,
	}
	result, err := investigation.NewService(c, c).StartFromThread(ctx, investigation.StartFromThreadInput{
		Baseline: investigation.ThreadBaseline{
			Repo: resolved.Repo, Kind: resolved.Kind, Number: resolved.Number,
			ObservationID: observation.ID, SourceUpdatedAt: observation.SourceUpdatedAt,
			ObservationSequence: observation.ObservationSequence, ObservedAt: observation.ObservedAt,
			Source: source, DescriptionTruncated: truncated,
		},
		Title: thread.Title, Description: description,
	})
	if err != nil {
		if errors.Is(err, investigation.ErrInvalidThreadBaseline) {
			return nil, err
		}
		return nil, fmt.Errorf("start investigation from thread: %w", err)
	}
	return &cli.ThreadInvestigationResult{
		Created: result.Created, Investigation: investigationResult(result.Investigation),
		Hypothesis: hypothesisResult(result.Hypothesis),
	}, nil
}

func boundedThreadSeedDescription(value string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if utf8.RuneCountInString(value) <= maxThreadSeedDescription {
		return value, false
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxThreadSeedDescription])) + "…", true
}

func threadBaselineResult(value *investigation.ThreadBaseline) *cli.ThreadBaselineResult {
	if value == nil {
		return nil
	}
	return &cli.ThreadBaselineResult{
		Ref: value.Ref(), Repository: value.Repo.String(), Kind: string(value.Kind), Number: value.Number,
		ObservationID: value.ObservationID, SourceUpdatedAt: formatTime(value.SourceUpdatedAt),
		ObservationSequence: value.ObservationSequence, ObservedAt: formatTime(value.ObservedAt),
		Source: workflowSourceRefResult(value.Source), DescriptionTruncated: value.DescriptionTruncated,
	}
}

func workflowSourceRefResult(value domain.SourceRef) cli.WorkflowSourceRefResult {
	return cli.WorkflowSourceRefResult{
		Source: value.Source, URL: value.URL, CommitSHA: value.CommitSHA,
		ObservedAt: formatTime(value.ObservedAt), AsOf: formatTime(value.AsOf),
	}
}

func workflowAuditResults(values []investigation.StatusChange) []cli.WorkflowAuditResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]cli.WorkflowAuditResult, len(values))
	for index, value := range values {
		result[index] = cli.WorkflowAuditResult{
			From: value.From, To: value.To, Rationale: value.Rationale, At: formatTime(value.At),
		}
	}
	return result
}

func workflowSourceRefResults(values []domain.SourceRef) []cli.WorkflowSourceRefResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]cli.WorkflowSourceRefResult, len(values))
	for index, value := range values {
		result[index] = workflowSourceRefResult(value)
	}
	return result
}

func workflowLinkResults(values []investigation.Link) []cli.WorkflowLinkResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]cli.WorkflowLinkResult, len(values))
	for index, value := range values {
		result[index] = cli.WorkflowLinkResult{Kind: value.Kind, Ref: value.Ref, Source: workflowSourceRefResult(value.Source)}
	}
	return result
}
