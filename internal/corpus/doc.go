// Package corpus provides the product-owned SQLite system of record.
//
// The corpus stores immutable source observations separately from normalized
// current-state projections. Projection writes reject stale source revisions,
// complete paginated facets replace prior children atomically, and local
// queries use deterministic ordering. Opening a corpus applies embedded Goose
// migrations and performs no network or process work.
package corpus
