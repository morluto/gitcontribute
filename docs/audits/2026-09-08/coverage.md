# External MCP invocation coverage — 2026-09-08

Every advertised tool was invoked. `returned` means a non-error response, not necessarily complete evidence. `queued` requires the terminal job outcomes in the audit report. Error counts include rejected test inputs and environmental failures. Raw logs and temporary runtime artifacts were removed after the follow-up validation at the user's request. Counts below preserve the original audit summary.

| Tool | Baseline calls | Candidate calls | Observed envelope/status |
| --- | ---: | ---: | --- |
| `code.index_repositories` | 1 | 0 | queued |
| `corpus.analyze_fix_patterns` | 1 | 0 | partial |
| `corpus.ensure_coverage` | 1 | 0 | queued |
| `corpus.explain_match` | 1 | 0 | returned |
| `corpus.find_clusters` | 1 | 0 | complete |
| `corpus.find_competing_pull_requests` | 1 | 1 | partial |
| `corpus.find_duplicates` | 1 | 1 | partial |
| `corpus.find_neighbors` | 1 | 0 | complete |
| `corpus.find_precedents` | 1 | 0 | complete |
| `corpus.find_pull_request_overlaps` | 1 | 0 | complete |
| `corpus.get_actor_facets` | 1 | 0 | returned |
| `corpus.get_actors` | 1 | 0 | returned |
| `corpus.get_coverage` | 1 | 0 | partial |
| `corpus.get_repositories` | 1 | 0 | returned |
| `corpus.get_thread_facets` | 1 | 0 | returned |
| `corpus.get_threads` | 2 | 0 | returned, tool_error |
| `corpus.list_concerns` | 1 | 0 | returned |
| `corpus.materialize_repository_dossier` | 1 | 0 | returned |
| `corpus.rank_contribution_candidates` | 1 | 0 | complete |
| `corpus.search_actors` | 1 | 0 | returned |
| `corpus.search_code` | 1 | 0 | partial |
| `corpus.search_contributions` | 1 | 0 | returned |
| `corpus.search_pull_request_feedback` | 1 | 0 | partial |
| `corpus.search_pull_requests` | 1 | 0 | partial |
| `corpus.search_repositories` | 1 | 0 | returned |
| `corpus.search_threads` | 1 | 3 | returned, tool_error |
| `evidence.import_manifest` | 1 | 0 | returned |
| `github.compare_fork` | 2 | 0 | current, tool_error |
| `github.get_authenticated_identity` | 1 | 0 | tool_error |
| `github.index_pull_request_feedback` | 1 | 0 | queued |
| `github.read_source_files` | 2 | 1 | complete, tool_error |
| `github.search_repositories` | 2 | 0 | complete, tool_error |
| `github.search_threads` | 1 | 0 | partial |
| `github.search_users` | 1 | 0 | returned |
| `github.sync_pull_request_ci` | 2 | 0 | queued, tool_error |
| `github.sync_pull_request_feedback` | 1 | 0 | queued |
| `github.sync_pull_request_portfolio` | 1 | 0 | queued |
| `github.sync_repository_context` | 1 | 0 | queued |
| `github.sync_thread_facets` | 1 | 0 | queued |
| `github.sync_threads` | 1 | 0 | queued |
| `github.sync_user_contributions` | 1 | 0 | queued |
| `github.sync_user_organizations` | 1 | 0 | queued |
| `github.sync_user_pinned_items` | 1 | 0 | queued |
| `github.sync_user_repositories` | 1 | 0 | queued |
| `github.sync_user_social_accounts` | 1 | 0 | queued |
| `github.sync_users` | 1 | 0 | queued |
| `github.wait_pull_request_checks` | 1 | 0 | queued |
| `jobs.cancel` | 1 | 0 | returned |
| `jobs.get` | 6 | 1 | protocol_error, returned |
| `research.query_deepwiki` | 1 | 0 | complete |
| `validation.attach_junit_report` | 1 | 0 | returned |
| `validation.attach_receipt` | 1 | 0 | returned |
| `validation.define` | 1 | 0 | returned |
| `validation.run` | 1 | 0 | queued |
| `workflow.create_concern` | 1 | 0 | returned |
| `workflow.export_manifest` | 2 | 0 | returned, tool_error |
| `workflow.get_catalog_contract` | 1 | 1 | returned |
| `workflow.get_source_audit_contract` | 1 | 0 | returned |
| `workflow.link_concern` | 1 | 0 | returned |
| `workflow.link_pull_request` | 1 | 0 | returned |
| `workflow.prepare_contribution` | 1 | 0 | returned |
| `workflow.promote_concern` | 1 | 0 | returned |
| `workflow.promote_opportunity` | 1 | 0 | returned |
| `workflow.record_hypothesis` | 1 | 0 | returned |
| `workflow.set_concern_status` | 1 | 0 | returned |
| `workflow.start_investigation` | 1 | 0 | returned |
| `workflow.update_concern` | 1 | 0 | returned |
| `workflow.verify_published_draft` | 1 | 0 | unknown |
| `workspace.adopt` | 2 | 0 | returned, tool_error |
| `workspace.check_merge_conflicts` | 1 | 0 | returned |
| `workspace.create` | 1 | 0 | queued |
| `workspace.inspect_commit_changes` | 1 | 0 | returned |
| `workspace.plan_semantic_commits` | 1 | 0 | returned |
