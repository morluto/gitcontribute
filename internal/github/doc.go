// Package github adapts read-only GitHub APIs to product-owned values.
//
// It contains credential resolution, typed error classification, per-attempt
// rate limiting, bounded retries, pagination metadata, and mapping from
// go-github types. Callers depend on the narrow Reader capabilities instead of
// importing SDK types into the application or domain layers.
package github
