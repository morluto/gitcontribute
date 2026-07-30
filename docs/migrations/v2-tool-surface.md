# Contribution operation migration

This release intentionally removes the scalar and compatibility contribution
surfaces. Existing durable jobs remain readable through `jobs.get`, but their
stored requests cannot be replayed through removed operations.

| Removed tool or command | Replacement |
| --- | --- |
| `github.get_authenticated_identity` | `github.sync_pull_request_portfolio` with `selection: authored` |
| `github.sync_authored_pull_requests` | `github.sync_pull_request_portfolio` with `selection: authored` |
| `github.sync_pull_request_status` | `github.sync_pull_request_portfolio` with `selection: explicit` |
| `github.sync_portfolio` | `github.sync_pull_request_portfolio` |
| `github.hydrate_threads` | `github.sync_thread_facets` |
| `corpus.list_pull_request_portfolio` | `corpus.list_pull_requests` |
| `corpus.find_portfolio_overlaps` | `corpus.find_pull_request_overlaps` |
| `corpus.rank_threads` | `corpus.rank_contribution_candidates` |
| `job show` | `jobs get` |
| `job cancel` | `jobs cancel` |

PR feedback and CI diagnostics are no longer incidental parts of generic
hydration. Use `github.sync_pull_request_feedback` for comments, reviews,
inline comments, and review-thread topology. Use `github.sync_ci_failures` for
current-head check and status observations. Both return durable jobs and retain
independent coverage; missing coverage is never a negative finding.

The removed names are not aliases. Calls fail as unknown operations so agents
cannot silently continue using the scalar workflow.
