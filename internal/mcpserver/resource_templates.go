package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

type resourceTemplateDefinition struct {
	template    string
	name        string
	description string
}

func (s *Server) registerResourceTemplates() {
	templates := []resourceTemplateDefinition{
		{"gitcontribute://repository/{owner}/{repo}", "Repository", "Local repository record"},
		{"gitcontribute://thread/{owner}/{repo}/{kind}/{number}", "Thread", "Local issue or pull request"},
		{"gitcontribute://dossier/{owner}/{repo}", "Dossier", "Local source-backed repository dossier"},
		{"gitcontribute://investigation/{id}", "Investigation", "Local investigation workspace"},
		{"gitcontribute://opportunity/{id}", "Opportunity", "Local contribution opportunity"},
		{"gitcontribute://evidence/{scope}/{id}", "Evidence", "Local evidence for an investigation or opportunity"},
		{"gitcontribute://readiness/{opportunity_id}", "Readiness", "Local contribution readiness report"},
		{"gitcontribute://lens/{name}", "Lens", "Saved lens definition"},
	}
	if _, ok := s.reader.(threadFacetResourceReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://thread/{owner}/{repo}/{kind}/{number}/facet/{facet}",
			name:     "Thread facet", description: "Persisted local thread facet payload",
		})
	}
	if _, ok := s.reader.(FixPatternReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://fix-pattern-report/{job_id}",
			name:     "Fix-pattern report", description: "Typed repository contribution-pattern report produced by a durable workflow",
		})
	}
	if _, ok := s.reader.(CodeIndexReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://artifact/code-index/{artifact_digest}",
			name:     "Code-index artifact", description: "Immutable digest-bound indexed repository commit manifest",
		})
	}
	if _, ok := s.reader.(SnapshotReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://snapshot/{snapshot_token}",
			name:     "Corpus snapshot", description: "Immutable scoped offline read artifact",
		})
	}
	if _, ok := s.reader.(concernResourceReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://concern/{id}",
			name:     "Concern", description: "Persisted local repository concern",
		})
	}
	if _, ok := s.reader.(draftResourceReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://draft/{id}/{revision}",
			name:     "Draft", description: "Immutable persisted contribution-draft revision",
		})
	}
	if _, ok := s.reader.(manifestResourceReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://manifest/{id}",
			name:     "Manifest", description: "Persisted contribution evidence manifest",
		})
	}
	if _, ok := s.reader.(workspaceResourceReader); ok {
		templates = append(templates, resourceTemplateDefinition{
			template: "gitcontribute://workspace/{id}",
			name:     "Workspace", description: "Host-path-free managed workspace with immutable recorded revisions",
		})
	}
	if _, ok := s.reader.(pullRequestWorkflowResourceReader); ok {
		templates = append(templates,
			resourceTemplateDefinition{
				template: "gitcontribute://pull-request-feedback/{owner}/{repo}/{number}",
				name:     "Pull-request feedback", description: "Persisted feedback channels with independent coverage",
			},
			resourceTemplateDefinition{
				template: "gitcontribute://ci-failure-report/{owner}/{repo}/{number}",
				name:     "CI failure report", description: "Persisted statuses, checks, workflow runs, and jobs",
			},
			resourceTemplateDefinition{
				template: "gitcontribute://ci-job-log/{owner}/{repo}/{number}/{job_id}",
				name:     "CI job log", description: "Bounded persisted log for one CI job",
			},
		)
	}
	addResourceTemplates(s, templates)
}

func addResourceTemplates(s *Server, templates []resourceTemplateDefinition) {
	for _, t := range templates {
		s.server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: t.template,
			Name:        t.name,
			Description: t.description,
			MIMEType:    "application/json",
		}, s.readResource)
	}
}
