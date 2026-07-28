package mcpcontract

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

// JobStatus describes the durable execution lifecycle exposed through MCP.
type JobStatus string

// JobExecutionState separates pollable execution from terminal completion.
type JobExecutionState string

// JobOutcome describes the result of a terminal durable job.
type JobOutcome string
