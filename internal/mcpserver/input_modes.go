package mcpserver

import "github.com/google/jsonschema-go/jsonschema"

const nonWhitespacePattern = `.*\S.*`

func configureRepositorySearchModes(builder *schemaBuilder) {
	structuredFields := []string{
		"text", "match_fields", "topics", "language", "stars_min", "stars_max",
		"created_after", "created_before", "pushed_after", "pushed_before",
		"archived", "fork",
	}
	rawQuery := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:github-search-raw-query",
		Required: []string{"raw_query"},
		Properties: map[string]*jsonschema.Schema{
			"raw_query": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny(structuredFields...),
	}
	structured := &jsonschema.Schema{
		ID:                "urn:gitcontribute:mode:github-search-structured",
		AnyOf:             schemasRequiringOne(structuredFields...),
		Not:               schemaRequiringAny("raw_query"),
		DependentRequired: map[string][]string{"match_fields": {"text"}},
	}
	builder.schema.OneOf = []*jsonschema.Schema{rawQuery, structured}
}

func configureSyncThreadModes(builder *schemaBuilder) {
	repositories := schemaMode("selection", "repositories",
		[]string{"repositories"},
		[]string{"threads"},
	)
	repositories.ID = "urn:gitcontribute:mode:sync-threads-repositories"
	repositories.Properties["repositories"] = &jsonschema.Schema{MinItems: jsonschema.Ptr(1)}

	threads := schemaMode("selection", "threads",
		[]string{"threads"},
		[]string{"repositories", "kind", "state", "updated_after", "limit_per_repository"},
	)
	threads.ID = "urn:gitcontribute:mode:sync-threads-exact"
	threads.Properties["threads"] = &jsonschema.Schema{MinItems: jsonschema.Ptr(1)}
	builder.schema.OneOf = []*jsonschema.Schema{repositories, threads}
}

func configureDeepWikiModes(builder *schemaBuilder) {
	structure := schemaMode("action", "structure",
		[]string{"repository"},
		[]string{"repositories", "question"},
	)
	structure.ID = "urn:gitcontribute:mode:deepwiki-structure"
	structure.Properties["repository"] = &jsonschema.Schema{MinLength: jsonschema.Ptr(1), Pattern: nonWhitespacePattern}

	contents := schemaMode("action", "contents",
		[]string{"repository"},
		[]string{"repositories", "question"},
	)
	contents.ID = "urn:gitcontribute:mode:deepwiki-contents"
	contents.Properties["repository"] = &jsonschema.Schema{MinLength: jsonschema.Ptr(1), Pattern: nonWhitespacePattern}

	question := schemaMode("action", "question",
		[]string{"repositories", "question"},
		[]string{"repository"},
	)
	question.ID = "urn:gitcontribute:mode:deepwiki-question"
	question.Properties["repositories"] = &jsonschema.Schema{
		MinItems: jsonschema.Ptr(1),
		Items:    &jsonschema.Schema{Pattern: nonWhitespacePattern},
	}
	question.Properties["question"] = &jsonschema.Schema{MinLength: jsonschema.Ptr(1), Pattern: nonWhitespacePattern}
	builder.schema.OneOf = []*jsonschema.Schema{structure, contents, question}
}

func configureContributionDraftModes(builder *schemaBuilder) {
	issue := schemaMode("kind", "issue", nil,
		[]string{"workspace_id", "approach", "changes", "compatibility", "limitations", "linked_issue"},
	)
	issue.ID = "urn:gitcontribute:mode:contribution-draft-issue"
	pullRequest := schemaMode("kind", "pull_request",
		[]string{"workspace_id", "approach"},
		[]string{"success"},
	)
	pullRequest.ID = "urn:gitcontribute:mode:contribution-draft-pull-request"
	pullRequest.Properties["workspace_id"] = &jsonschema.Schema{MinLength: jsonschema.Ptr(1)}
	pullRequest.Properties["approach"] = &jsonschema.Schema{MinLength: jsonschema.Ptr(1)}
	builder.schema.OneOf = []*jsonschema.Schema{issue, pullRequest}
}

func configureInvestigationSourceModes(builder *schemaBuilder) {
	setEnum(builder, "kind", "issue", "pull_request")
	commit := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:investigation-commit",
		Required: []string{"commit_sha"},
		Properties: map[string]*jsonschema.Schema{
			"commit_sha": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny("kind", "number"),
	}
	thread := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:investigation-thread",
		Required: []string{"number"},
		Properties: map[string]*jsonschema.Schema{
			"number": {Minimum: jsonschema.Ptr(1.0)},
		},
		Not: schemaRequiringAny("commit_sha", "lens"),
	}
	builder.schema.OneOf = []*jsonschema.Schema{commit, thread}
}

func configureConcernSourceModes(builder *schemaBuilder) {
	commit := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:concern-commit",
		Required: []string{"commit_sha"},
		Properties: map[string]*jsonschema.Schema{
			"commit_sha": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny("workspace_id"),
	}
	workspace := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:concern-workspace",
		Required: []string{"workspace_id"},
		Properties: map[string]*jsonschema.Schema{
			"workspace_id": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny("commit_sha"),
	}
	builder.schema.OneOf = []*jsonschema.Schema{commit, workspace}
}

func configureValidationWorkspaceModes(builder *schemaBuilder) {
	workspace := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:validation-workspace",
		Required: []string{"workspace_id"},
		Properties: map[string]*jsonschema.Schema{
			"workspace_id": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny("base_workspace_id", "candidate_workspace_id"),
	}
	comparison := &jsonschema.Schema{
		ID:       "urn:gitcontribute:mode:validation-comparison",
		Required: []string{"base_workspace_id", "candidate_workspace_id"},
		Properties: map[string]*jsonschema.Schema{
			"base_workspace_id":      {MinLength: jsonschema.Ptr(1)},
			"candidate_workspace_id": {MinLength: jsonschema.Ptr(1)},
		},
		Not: schemaRequiringAny("workspace_id"),
	}
	builder.schema.OneOf = []*jsonschema.Schema{workspace, comparison}
}

func schemaMode(discriminator, value string, required, forbidden []string) *jsonschema.Schema {
	constValue := any(value)
	schema := &jsonschema.Schema{
		Required: append([]string{discriminator}, required...),
		Properties: map[string]*jsonschema.Schema{
			discriminator: {Const: &constValue},
		},
	}
	if len(forbidden) > 0 {
		schema.Not = schemaRequiringAny(forbidden...)
	}
	return schema
}

func schemaRequiringAny(fields ...string) *jsonschema.Schema {
	return &jsonschema.Schema{AnyOf: schemasRequiringOne(fields...)}
}

func schemasRequiringOne(fields ...string) []*jsonschema.Schema {
	schemas := make([]*jsonschema.Schema, len(fields))
	for i, field := range fields {
		schemas[i] = &jsonschema.Schema{Required: []string{field}}
	}
	return schemas
}
