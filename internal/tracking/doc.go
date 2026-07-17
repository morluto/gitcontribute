// Package tracking models local contribution research history.
//
// Triage decisions, prepared contributions, and outcomes are validated before
// persistence and can be exported deterministically. Export sanitization
// redacts credentials and absolute local paths; it is a publication boundary,
// not a substitute for avoiding secrets in source records.
package tracking
