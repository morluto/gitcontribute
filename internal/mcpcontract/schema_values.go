package mcpcontract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Probability is a numeric confidence value in the inclusive range [0, 1].
type Probability float64

// SimilarityScore is a normalized similarity value in the inclusive range [0, 1].
type SimilarityScore float64

// RadarScore is a deterministic Contribution Radar score in the inclusive
// range [0, 100].
type RadarScore int

// ProgressPercent is an integer completion percentage in the inclusive range
// [0, 100].
type ProgressPercent int

// NonNegativeInt is an integer count or delay that cannot be negative.
type NonNegativeInt int

// BatchItemStatus describes the outcome of one item in a bounded batch.
type BatchItemStatus string

const (
	BatchItemComplete    BatchItemStatus = "complete"
	BatchItemPartial     BatchItemStatus = "partial"
	BatchItemRetryable   BatchItemStatus = "retryable"
	BatchItemUnavailable BatchItemStatus = "unavailable"
	BatchItemFailed      BatchItemStatus = "failed"
)

// ParseBatchItemStatus canonicalizes one item outcome from a durable or wire
// representation before it enters application logic.
func ParseBatchItemStatus(value string) (BatchItemStatus, error) {
	switch BatchItemStatus(strings.ToLower(strings.TrimSpace(value))) {
	case BatchItemComplete:
		return BatchItemComplete, nil
	case BatchItemPartial:
		return BatchItemPartial, nil
	case BatchItemRetryable:
		return BatchItemRetryable, nil
	case BatchItemUnavailable:
		return BatchItemUnavailable, nil
	case BatchItemFailed:
		return BatchItemFailed, nil
	default:
		return "", fmt.Errorf("unsupported batch item status %q", value)
	}
}

func (s *BatchItemStatus) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseBatchItemStatus(value)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// JobStatus describes the durable execution lifecycle exposed through MCP.
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// JobExecutionState separates pollable execution from terminal completion.
type JobExecutionState string

const (
	JobExecutionQueued   JobExecutionState = "queued"
	JobExecutionRunning  JobExecutionState = "running"
	JobExecutionTerminal JobExecutionState = "terminal"
)

// JobOutcome describes the result of a terminal durable job.
type JobOutcome string

const (
	JobOutcomeSucceeded JobOutcome = "succeeded"
	JobOutcomePartial   JobOutcome = "partial"
	JobOutcomeFailed    JobOutcome = "failed"
	JobOutcomeCancelled JobOutcome = "cancelled"
)
