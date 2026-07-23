package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxRepeatRuns        = 100
	maxRepeatConcurrency = 16
	minSampleInterval    = 10 * time.Millisecond
	maxSampleInterval    = 10 * time.Second
)

type validationTask struct {
	index int
	kind  RunKind
}

type validationTaskResult struct {
	attempt ValidationAttempt
	run     *ValidationRun
}

// RunValidationGroup executes independently timed attempts, persists every
// successful run record, and then persists one bounded aggregate.
func (s *Service) RunValidationGroup(ctx context.Context, defID string, opts RepeatValidationOptions) (*ValidationRunGroup, error) {
	def, err := s.repo.GetValidationDefinition(ctx, defID)
	if err != nil {
		return nil, err
	}
	opts, err = normalizeRepeatOptions(def, opts)
	if err != nil {
		return nil, err
	}
	configurationDigest, err := validationConfigurationDigest(def)
	if err != nil {
		return nil, err
	}
	group := &ValidationRunGroup{
		ID: uuid.NewString(), DefinitionID: def.ID, InvestigationID: def.InvestigationID,
		HypothesisID: def.HypothesisID, OpportunityID: def.OpportunityID,
		ConfigurationSHA256: configurationDigest, RequestedRuns: len(opts.Kinds) * opts.RunCount,
		Concurrency: opts.Concurrency, PerRunTimeout: opts.PerRunTimeout, OverallTimeout: opts.OverallTimeout,
		SampleInterval: opts.SampleInterval, StartedAt: time.Now().UTC(),
	}
	groupCtx, cancel := context.WithTimeout(ctx, opts.OverallTimeout)
	defer cancel()
	tasks := repeatTasks(opts.Kinds, opts.RunCount)
	results := s.executeValidationTasks(groupCtx, def, opts, tasks)
	for result := range results {
		group.Attempts = append(group.Attempts, result.attempt)
		if result.run != nil {
			group.CompletedRuns++
		}
	}
	sort.Slice(group.Attempts, func(i, j int) bool {
		if group.Attempts[i].Kind != group.Attempts[j].Kind {
			return group.Attempts[i].Kind == RunKindBase
		}
		return group.Attempts[i].Index < group.Attempts[j].Index
	})
	group.CompletedAt = time.Now().UTC()
	group.Aggregates = aggregateAttempts(group.Attempts, opts.Kinds, opts.RunCount, def.Observation != nil)
	group.Classification = aggregateGroupClassification(group.Aggregates)
	group.Comparison = compareValidationAggregates(group.Aggregates)
	saveCtx := ctx
	if ctx.Err() != nil {
		var saveCancel context.CancelFunc
		saveCtx, saveCancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer saveCancel()
	}
	if err := s.repo.SaveValidationRunGroup(saveCtx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func normalizeRepeatOptions(def *ValidationDefinition, opts RepeatValidationOptions) (RepeatValidationOptions, error) {
	if opts.RunCount < 1 || opts.RunCount > maxRepeatRuns {
		return opts, fmt.Errorf("repeat run count must be between 1 and %d", maxRepeatRuns)
	}
	if len(opts.Kinds) == 0 || len(opts.Kinds) > 2 {
		return opts, errors.New("one or two validation kinds are required")
	}
	seen := map[RunKind]bool{}
	for _, kind := range opts.Kinds {
		if (kind != RunKindBase && kind != RunKindCandidate) || seen[kind] {
			return opts, ErrMissingRunKind
		}
		seen[kind] = true
	}
	total := len(opts.Kinds) * opts.RunCount
	if opts.Concurrency < 1 || opts.Concurrency > maxRepeatConcurrency || opts.Concurrency > total {
		return opts, fmt.Errorf("repeat concurrency must be between 1 and %d and no greater than total attempts", maxRepeatConcurrency)
	}
	if opts.PerRunTimeout == 0 {
		opts.PerRunTimeout = def.Timeout
	}
	if opts.PerRunTimeout <= 0 || opts.PerRunTimeout > maxValidationTimeout {
		return opts, ErrInvalidTimeout
	}
	if opts.SampleInterval == 0 {
		opts.SampleInterval = defaultSampleInterval
	}
	if opts.SampleInterval < minSampleInterval || opts.SampleInterval > maxSampleInterval {
		return opts, fmt.Errorf("sample interval must be between %s and %s", minSampleInterval, maxSampleInterval)
	}
	if opts.OverallTimeout == 0 {
		waves := (total + opts.Concurrency - 1) / opts.Concurrency
		opts.OverallTimeout = time.Duration(waves) * opts.PerRunTimeout
	}
	if opts.OverallTimeout <= 0 || opts.OverallTimeout > maxValidationTimeout {
		return opts, ErrInvalidTimeout
	}
	return opts, nil
}

func repeatTasks(kinds []RunKind, count int) []validationTask {
	tasks := make([]validationTask, 0, len(kinds)*count)
	for _, kind := range kinds {
		for index := 1; index <= count; index++ {
			tasks = append(tasks, validationTask{index: index, kind: kind})
		}
	}
	return tasks
}

func (s *Service) executeValidationTasks(ctx context.Context, def *ValidationDefinition, opts RepeatValidationOptions, pending []validationTask) <-chan validationTaskResult {
	tasks := make(chan validationTask, len(pending))
	results := make(chan validationTaskResult, len(pending))
	for _, task := range pending {
		tasks <- task
	}
	close(tasks)
	var workers sync.WaitGroup
	workers.Add(opts.Concurrency)
	for range opts.Concurrency {
		go func() {
			defer workers.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case task, ok := <-tasks:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					results <- s.executeValidationTask(ctx, def, opts, task)
				}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()
	return results
}

func (s *Service) executeValidationTask(ctx context.Context, def *ValidationDefinition, opts RepeatValidationOptions, task validationTask) validationTaskResult {
	startedAt := time.Now().UTC()
	run, err := s.runDefinition(ctx, def, task.kind, opts.PerRunTimeout, opts.SampleInterval)
	if err != nil {
		classification := RunClassificationError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			classification = RunClassificationCancelled
		}
		return validationTaskResult{attempt: ValidationAttempt{Index: task.index, Kind: task.kind, StartedAt: startedAt, CompletedAt: time.Now().UTC(), Classification: classification, Error: err.Error()}}
	}
	return validationTaskResult{run: run, attempt: ValidationAttempt{
		Index: task.index, Kind: task.kind, RunID: run.ID, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
		ExitCode: run.ExitCode, Classification: run.Classification, ObservationStatus: run.ObservationStatus,
		TimeoutPhase: run.TimeoutPhase, FailurePhase: run.FailurePhase,
		Error: run.Error, Process: run.Process, Phases: run.Phases,
		Resources: run.Resources, Cleanup: run.Cleanup,
	}}
}

func aggregateAttempts(attempts []ValidationAttempt, kinds []RunKind, requested int, hasObservations bool) []ValidationAggregate {
	aggregates := make([]ValidationAggregate, 0, len(kinds))
	for _, kind := range kinds {
		aggregate := ValidationAggregate{Kind: kind, Requested: requested, ResourceClassification: "available"}
		for _, attempt := range attempts {
			if attempt.Kind != kind {
				continue
			}
			aggregate.Completed++
			if attempt.Cleanup.Status == "failed" {
				aggregate.ResourceClassification = "cleanup_failed"
			} else if aggregate.ResourceClassification == "available" && resourcesUnavailable(attempt.Resources) {
				aggregate.ResourceClassification = "inconclusive"
			}
			switch semanticAttempt(attempt, hasObservations) {
			case RunGroupStablePass:
				aggregate.Passing++
			case RunGroupStableFail:
				aggregate.Failing++
			case RunGroupCancelled:
				aggregate.Cancelled++
			default:
				aggregate.Inconclusive++
			}
		}
		aggregate.Classification = classifyAggregate(aggregate)
		aggregates = append(aggregates, aggregate)
	}
	return aggregates
}

func semanticAttempt(attempt ValidationAttempt, hasObservations bool) RunGroupClassification {
	if attempt.Classification == RunClassificationCancelled {
		return RunGroupCancelled
	}
	if attempt.Classification == RunClassificationError || attempt.RunID == "" {
		return RunGroupInconclusive
	}
	if hasObservations && attempt.ObservationStatus != ObservationMatched {
		return RunGroupInconclusive
	}
	if attempt.Classification == RunClassificationPassing {
		return RunGroupStablePass
	}
	if attempt.Classification == RunClassificationFailing {
		return RunGroupStableFail
	}
	return RunGroupInconclusive
}

func classifyAggregate(aggregate ValidationAggregate) RunGroupClassification {
	if aggregate.Inconclusive > 0 {
		return RunGroupInconclusive
	}
	if aggregate.Passing > 0 && aggregate.Failing > 0 {
		return RunGroupFlaky
	}
	if aggregate.Cancelled > 0 || aggregate.Completed < aggregate.Requested {
		return RunGroupCancelled
	}
	if aggregate.Passing == aggregate.Requested {
		return RunGroupStablePass
	}
	if aggregate.Failing == aggregate.Requested {
		return RunGroupStableFail
	}
	return RunGroupInconclusive
}

func aggregateGroupClassification(aggregates []ValidationAggregate) RunGroupClassification {
	if len(aggregates) == 1 {
		return aggregates[0].Classification
	}
	for _, aggregate := range aggregates {
		if aggregate.Classification == RunGroupCancelled {
			return RunGroupCancelled
		}
		if aggregate.Classification == RunGroupFlaky || aggregate.Classification == RunGroupInconclusive {
			return RunGroupInconclusive
		}
	}
	return RunGroupInconclusive
}

func compareValidationAggregates(aggregates []ValidationAggregate) *ValidationGroupComparison {
	if len(aggregates) != 2 {
		return nil
	}
	byKind := map[RunKind]ValidationAggregate{aggregates[0].Kind: aggregates[0], aggregates[1].Kind: aggregates[1]}
	base, baseOK := byKind[RunKindBase]
	candidate, candidateOK := byKind[RunKindCandidate]
	if !baseOK || !candidateOK {
		return nil
	}
	if base.Classification == RunGroupStableFail && candidate.Classification == RunGroupStablePass {
		return &ValidationGroupComparison{Classification: ComparisonFixed, Explanation: "base fails consistently and candidate passes consistently"}
	}
	if base.Classification == RunGroupStableFail && candidate.Classification == RunGroupStableFail {
		return &ValidationGroupComparison{Classification: ComparisonNotFixed, Explanation: "base and candidate fail consistently"}
	}
	if base.Classification == RunGroupStablePass && candidate.Classification == RunGroupStablePass {
		return &ValidationGroupComparison{Classification: ComparisonNoDifference, Explanation: "base and candidate pass consistently"}
	}
	if base.Classification == RunGroupStablePass && candidate.Classification == RunGroupStableFail {
		return &ValidationGroupComparison{Classification: ComparisonRegression, Explanation: "base passes consistently and candidate fails consistently"}
	}
	return &ValidationGroupComparison{Classification: ComparisonInconclusive, Explanation: "flaky, cancelled, or semantically incomparable attempts prevent a fixed/not-fixed conclusion"}
}

func resourcesUnavailable(resources ResourceTelemetry) bool {
	return resources.CPUTimeMillis.Value == nil || resources.PeakRSSBytes.Value == nil || resources.PeakChildCount.Value == nil
}

func validationConfigurationDigest(def *ValidationDefinition) (string, error) {
	payload, err := json.Marshal(struct {
		Command          []string
		Environment      []string
		Timeout          time.Duration
		MaxOutputBytes   int64
		Observation      *ObservationContract
		Protocol         ValidationProtocol
		ReadinessTimeout time.Duration
	}{def.Command, def.Env, def.Timeout, def.MaxOutputBytes, def.Observation, def.Protocol, def.ReadinessTimeout})
	if err != nil {
		return "", fmt.Errorf("encode validation configuration: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
