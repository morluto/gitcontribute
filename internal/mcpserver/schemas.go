package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

var outputPropertyDescriptions = map[string]string{
	"id":                        "Stable identifier used by related GitContribute tools and resources.",
	"code":                      "Stable machine-readable warning or error code.",
	"name":                      "Human-readable stable name.",
	"owner":                     "GitHub repository owner.",
	"repo":                      "GitHub repository name.",
	"kind":                      "Domain-specific record or operation kind.",
	"status":                    "Current domain-specific status.",
	"state":                     "Current GitHub or local workflow state.",
	"number":                    "GitHub issue or pull request number.",
	"title":                     "Human-readable title.",
	"body":                      "Stored issue, pull request, or draft body.",
	"author":                    "GitHub login of the author.",
	"labels":                    "Stored GitHub labels.",
	"query":                     "Search query used to produce this result.",
	"interpretation":            "Concise explanation of how the request was interpreted.",
	"response_format":           "Response detail level used for this result.",
	"page":                      "One-based provider result page.",
	"next_page":                 "Next provider result page; absent when no next page is available.",
	"warnings":                  "Request-specific limitations that affect interpretation.",
	"suggestion":                "Actionable adjustment that addresses the warning.",
	"suggested_actions":         "Non-mandatory follow-up tool calls with reusable arguments.",
	"tool":                      "Canonical GitContribute tool name for a suggested action.",
	"arguments":                 "Arguments for the suggested tool call.",
	"total":                     "Total number of matching records represented by this response.",
	"limit":                     "Maximum number of records requested.",
	"matches":                   "Ordered matches for the current page.",
	"next_cursor":               "Opaque cursor for the next page; absent when there is no next page.",
	"fields":                    "Stored repository fields from the winning local projection.",
	"sections":                  "Source-backed dossier sections keyed by section name.",
	"as_of":                     "RFC 3339 timestamp describing how current the response is.",
	"updated_at":                "RFC 3339 timestamp of the latest update.",
	"created_at":                "RFC 3339 timestamp when the record was created.",
	"started_at":                "RFC 3339 timestamp when execution started.",
	"completed_at":              "RFC 3339 timestamp when execution completed.",
	"cancelled_at":              "RFC 3339 timestamp when cancellation was requested or completed.",
	"observed_at":               "RFC 3339 timestamp when source data was observed.",
	"rendered_at":               "RFC 3339 timestamp when the draft was rendered.",
	"evaluated_at":              "RFC 3339 timestamp when the rule was evaluated.",
	"commit":                    "Git commit SHA for the indexed snapshot.",
	"commit_sha":                "Git commit SHA associated with the record.",
	"path":                      "Repository-relative file path.",
	"language":                  "Detected source language.",
	"snippet":                   "Relevant bounded excerpt from the stored source.",
	"bytes":                     "Size of the indexed content in bytes.",
	"score":                     "Deterministic normalized score from 0.0 to 1.0.",
	"confidence":                "Confidence value from 0.0 to 1.0.",
	"reason":                    "Human-readable explanation grounded in stored facts.",
	"source_revision":           "Opaque revision of the local source projection used by the result.",
	"source_refs":               "Source references supporting this record.",
	"source_provenance":         "Stored source revisions used to derive the evidence.",
	"matched_fields":            "Fields that contributed to the match.",
	"facets":                    "Facet coverage or hydration results.",
	"facet":                     "Stored GitHub facet name.",
	"complete":                  "Whether retrieval completed without a page cap or interruption.",
	"count":                     "Number of records represented by this item.",
	"pages":                     "Number of GitHub pages retrieved.",
	"requests":                  "Number of external requests performed.",
	"message":                   "Human-readable operation summary.",
	"investigation_id":          "Investigation identifier.",
	"opportunity_id":            "Contribution opportunity identifier.",
	"hypothesis_id":             "Hypothesis identifier.",
	"hypothesis_total":          "Total hypotheses associated with the investigation.",
	"hypotheses":                "Bounded hypothesis summaries.",
	"opportunities":             "Bounded contribution opportunity summaries.",
	"evidence":                  "Bounded evidence records.",
	"evidence_total":            "Total evidence records associated with the opportunity.",
	"evidence_ids":              "Bounded evidence identifiers for follow-up reads.",
	"findings":                  "Evidence-backed duplicate or collision candidates.",
	"clusters":                  "Duplicate-candidate clusters.",
	"neighbors":                 "Similar stored threads ordered by score.",
	"canonical":                 "Canonical member selected for the cluster.",
	"members":                   "Cluster members.",
	"member_count":              "Total members in the cluster.",
	"included":                  "Whether the member passed the cluster inclusion threshold.",
	"problem_statement":         "Concrete contribution problem statement.",
	"scope":                     "Intended contribution scope.",
	"impact":                    "Expected user or project impact.",
	"expected_effort":           "Estimated implementation effort.",
	"dependencies":              "Known dependencies or prerequisites.",
	"maintainer_alignment":      "Recorded maintainer-alignment context; not proof of approval.",
	"collision_status":          "Assessment of potentially competing open work.",
	"description":               "Human-readable record description.",
	"category":                  "Contribution hypothesis category.",
	"expected_behavior":         "Expected behavior stated by the investigation.",
	"observed_behavior":         "Observed behavior stated by the investigation.",
	"potential_impact":          "Potential impact stated by the investigation.",
	"open_questions":            "Questions that remain unresolved.",
	"affected_components":       "Repository components believed to be affected.",
	"relation":                  "How this evidence relates to the hypothesis or opportunity.",
	"freshness":                 "Derived evidence freshness classification.",
	"freshness_reason":          "Explanation for the freshness classification.",
	"rule_set_version":          "Version of the deterministic readiness rule set.",
	"checks":                    "Individual deterministic readiness checks.",
	"check_id":                  "Stable readiness check identifier.",
	"rule_id":                   "Stable rule identifier.",
	"rule_version":              "Version of the evaluated rule.",
	"summary":                   "Concise explanation of the result.",
	"evidence_refs":             "Evidence references supporting the result.",
	"remediation":               "Smallest suggested action for a non-pass result.",
	"request":                   "Structured request submitted for the durable job.",
	"result":                    "Structured result produced by a completed durable job.",
	"error":                     "Failure message for an unsuccessful job.",
	"phase":                     "Current durable job phase.",
	"completed_items":           "Items completed in the current phase.",
	"total_items":               "Total items in the current phase.",
	"progress_percent":          "Integer completion percentage.",
	"cancellation_requested":    "Whether durable cancellation has been requested.",
	"checks_status":             "Derived check-rollup state from complete stored check coverage.",
	"checks_total":              "Number of checks in the complete stored head-revision snapshot.",
	"unresolved_review_threads": "Count of current unresolved, non-outdated review conversations when coverage is complete.",
	"merge_state_status":        "Detailed GitHub merge-state status when observed.",
	"merge_queue_state":         "Observed merge-queue entry state; absent only when complete coverage confirms no entry.",
	"merge_queue_position":      "Observed position of the pull request in the merge queue.",
	"closing_issues":            "Issues GitHub reports will be closed by this pull request.",
	"changed_files":             "Complete stored set of changed repository-relative paths.",
	"command":                   "Shell-free command and arguments.",
	"working_dir":               "Workspace directory used for validation.",
	"base_working_dir":          "Base workspace directory used for comparison validation.",
	"candidate_dir":             "Candidate workspace directory used for comparison validation.",
	"environment_allowlist":     "Environment variable names allowed during validation.",
	"timeout":                   "Maximum validation duration as a Go duration string.",
	"max_output_bytes":          "Maximum captured validation output in bytes.",
	"definition":                "Saved lens definition.",
	"updated":                   "Number of local projections updated by the operation.",
	"stable_id":                 "Stable cluster identifier.",
	"observation_sequence":      "Monotonic local observation sequence.",
	"source_updated_at":         "RFC 3339 timestamp reported by the source.",
	"thread_kind":               "Stored thread kind: issue or pull_request.",
	"lens":                      "Saved lens name associated with the investigation.",
	"target":                    "Record type checked: hypothesis or opportunity.",
	"type":                      "Domain-specific evidence or source type.",
	"subject":                   "Stored corpus subject whose revision is tracked.",
	"filter":                    "Saved lens filters used to select candidate records.",
	"languages":                 "Repository languages accepted by the saved lens.",
	"exclude_archived":          "Whether archived repositories are excluded.",
	"unassigned":                "Whether results must be unassigned.",
	"updated_within":            "Maximum age accepted by the saved lens.",
	"min_stars":                 "Minimum repository star count accepted by the saved lens.",
	"kinds":                     "Thread kinds accepted by the saved lens.",
	"states":                    "Thread states accepted by the saved lens.",
	"weights":                   "Signal weights applied by the saved lens.",
	"max_results_per_repo":      "Maximum lens-ranked results returned for one repository.",
	"items":                     "Ordered per-input results with item-level status.",
	"key":                       "Stable input-derived identity for this batch item.",
	"value":                     "Successful value for this batch item.",
	"retry_after_ms":            "Suggested milliseconds to wait before retrying this item.",
	"poll_after_ms":             "Suggested milliseconds to wait before polling the durable job.",
	"next_action":               "Suggested next tool or recovery action.",
	"metadata":                  "Repository metadata facet coverage and provenance.",
	"default_branch":            "Repository default branch reported by GitHub.",
	"license":                   "Repository license identifier reported by GitHub.",
	"topics":                    "Repository topics reported by GitHub.",
	"stars":                     "GitHub stargazer count when metadata is observed.",
	"watchers":                  "GitHub watcher count when metadata is observed.",
	"forks":                     "GitHub fork count when metadata is observed.",
	"open_issues":               "GitHub open issue and pull-request count when metadata is observed.",
	"archived":                  "Whether GitHub reports the repository as archived.",
	"fork":                      "Whether GitHub reports the repository as a fork.",
	"candidates":                "Compact ranked contribution candidates.",
	"candidate":                 "Candidate subject associated with this ordered overlap result.",
	"coverage":                  "Explicit completeness state for each evidence facet.",
	"pull_request_thread_id":    "Local corpus thread identity for the authored pull request.",
	"repositories":              "Repository identities or per-repository batch results.",
	"rank":                      "One-based position in the returned ranking.",
	"ref":                       "Stable repository or thread reference.",
	"url":                       "Canonical source URL for the record.",
	"eligibility":               "Objective candidate eligibility: eligible, unknown, or blocked.",
	"positive_signals":          "Stored facts that increased the candidate score.",
	"risks":                     "Stored facts that indicate contribution risk.",
	"blockers":                  "Stored facts that objectively block the contribution.",
	"unknowns":                  "Missing evidence that was not treated as negative.",
	"linked_pull_requests":      "Stored pull requests explicitly linked to the candidate.",
	"total_open_issues":         "Total stored open issues in the repository.",
	"considered":                "Bounded candidate population considered for ranking.",
	"generated_at":              "RFC 3339 timestamp when this derived result was generated.",
	"provider":                  "External provider that produced derived context.",
	"action":                    "Provider operation used for this result.",
	"question":                  "Exact question sent to the external provider.",
	"source_url":                "Provider or source URL supporting this result.",
	"retrieved_at":              "RFC 3339 timestamp when external context was retrieved.",
	"provenance":                "Source class distinguishing facts from derived external prose.",
	"truncated":                 "Whether the result was shortened to its requested byte bound.",
	"state_reason":              "GitHub reason associated with a closed thread state.",
	"author_association":        "GitHub association of the thread author with the repository.",
	"assignees":                 "GitHub logins assigned to the thread.",
	"draft":                     "Whether GitHub reports the pull request as draft.",
	"merged":                    "Whether GitHub reports the pull request as merged.",
	"login":                     "Authenticated GitHub login.",
	"node_id":                   "Stable GitHub GraphQL node identifier.",
	"pull_requests":             "Bounded pull requests represented by this result.",
	"attention":                 "Primary deterministic contributor attention state.",
	"mergeable":                 "GitHub mergeability when computation completed.",
	"head_ref":                  "Pull-request head branch name.",
	"head_sha":                  "Observed pull-request head commit OID.",
	"base_ref":                  "Pull-request base branch name.",
	"base_sha":                  "Observed pull-request base commit OID.",
	"review_decision":           "Decision derived from latest stored reviews per reviewer.",
	"status_coverage":           "Completeness of stored pull-request status facets.",
	"status_observed_at":        "RFC 3339 timestamp when status details were observed.",
	"workspace_id":              "Managed local workspace identifier.",
	"base_oid":                  "Already-fetched base Git object ID.",
	"head_oid":                  "Already-fetched head Git object ID.",
	"merge_base":                "Git merge-base object ID used for comparison.",
	"conflicted":                "Whether the local Git comparison found merge conflicts.",
	"source":                    "Input reference or provider source associated with this result.",
	"reasons":                   "Evidence-backed explanations for the result.",
	"closed_at":                 "RFC 3339 timestamp when GitHub reports the thread closed.",
	"merged_at":                 "RFC 3339 timestamp when GitHub reports the pull request merged.",
	"incomplete":                "Whether GitHub reported that the bounded search result may be incomplete.",
	"pushed_at":                 "RFC 3339 timestamp of the latest repository push reported by GitHub.",
}

func inferredSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("infer MCP schema: %v", err))
	}
	return schema
}

func inputSchema[T any](customize func(*jsonschema.Schema)) *jsonschema.Schema {
	schema := inferredSchema[T]()
	if customize != nil {
		customize(schema)
	}
	return schema
}

func outputSchema[T any](description string) *jsonschema.Schema {
	schema := inferredSchema[T]()
	schema.Description = description
	describeOutputProperties(schema)
	return schema
}

func describeOutputProperties(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties["key"] != nil && schema.Properties["value"] != nil && schema.Properties["reason"] != nil {
		setEnum(schema, "status", "complete", "retryable", "unavailable", "failed")
	}
	if schema.Properties["progress_percent"] != nil {
		setEnum(schema, "status", "queued", "running", "succeeded", "failed", "cancelled")
		setRange(schema, "progress_percent", 0, 100)
		setMinimum(schema, "completed_items", 0)
		setMinimum(schema, "total_items", 0)
		setMinimum(schema, "retry_after_ms", 0)
	}
	for name, property := range schema.Properties {
		if property.Description == "" {
			property.Description = outputPropertyDescriptions[name]
		}
		if strings.HasSuffix(name, "_at") || name == "as_of" {
			property.Format = "date-time"
		}
		if name == "score" || name == "confidence" {
			property.Minimum = jsonschema.Ptr(0.0)
			property.Maximum = jsonschema.Ptr(1.0)
		}
		describeOutputProperties(property)
		describeOutputProperties(property.Items)
		describeOutputProperties(property.AdditionalProperties)
	}
	for _, definition := range schema.Defs {
		describeOutputProperties(definition)
	}
}

func property(schema *jsonschema.Schema, name string) *jsonschema.Schema {
	p := schema.Properties[name]
	if p == nil {
		panic(fmt.Sprintf("MCP schema property %q not found", name))
	}
	return p
}

func setEnum(schema *jsonschema.Schema, name string, values ...string) {
	p := property(schema, name)
	p.Enum = make([]any, len(values))
	for i, value := range values {
		p.Enum[i] = value
	}
}

func setArrayEnum(schema *jsonschema.Schema, name string, values ...string) {
	p := property(schema, name)
	if p.Items == nil {
		panic(fmt.Sprintf("MCP schema array property %q has no items schema", name))
	}
	p.Items.Enum = make([]any, len(values))
	for i, value := range values {
		p.Items.Enum[i] = value
	}
}

func setRange(schema *jsonschema.Schema, name string, minimum, maximum float64) {
	p := property(schema, name)
	p.Minimum = jsonschema.Ptr(minimum)
	p.Maximum = jsonschema.Ptr(maximum)
}

func setMinimum(schema *jsonschema.Schema, name string, minimum float64) {
	property(schema, name).Minimum = jsonschema.Ptr(minimum)
}

func setDefault(schema *jsonschema.Schema, name string, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal MCP schema default for %q: %v", name, err))
	}
	property(schema, name).Default = b
}

func setConst(schema *jsonschema.Schema, name string, value any) {
	property(schema, name).Const = &value
}

func requireTogether(schema *jsonschema.Schema, names ...string) {
	if schema.DependentRequired == nil {
		schema.DependentRequired = make(map[string][]string)
	}
	for _, name := range names {
		others := make([]string, 0, len(names)-1)
		for _, other := range names {
			if other != name {
				others = append(others, other)
			}
		}
		schema.DependentRequired[name] = others
	}
}

func requireExactlyOne(schema *jsonschema.Schema, first, second string) {
	schema.OneOf = []*jsonschema.Schema{
		{Required: []string{first}, Not: &jsonschema.Schema{Required: []string{second}}},
		{Required: []string{second}, Not: &jsonschema.Schema{Required: []string{first}}},
	}
}

func setPositiveItems(schema *jsonschema.Schema, name string) {
	p := property(schema, name)
	if p.Items == nil {
		panic(fmt.Sprintf("MCP schema array property %q has no items schema", name))
	}
	p.Items.Minimum = jsonschema.Ptr(1.0)
}
