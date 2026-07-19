// Package clustering computes exact duplicate-thread clusters from caller-owned
// candidate snapshots.
//
// Computation is deterministic, cancellable, bounded by an ExactPairBudget,
// and free of storage and network side effects. Durable projection lifecycle,
// governance, and stale-safe atomic persistence belong to the corpus and
// clusterprojection packages.
package clustering
