// Package discovery turns explicit repositories, GitHub repository searches,
// and GH Archive events into bounded discovery signals.
//
// Search partitioning and archive readers are cancellation-aware and expose
// checkpoint state to callers. This package identifies candidates; the
// application and corpus layers own network authorization and persistence.
package discovery
