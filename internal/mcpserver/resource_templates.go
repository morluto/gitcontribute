package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

func (s *Server) registerResourceTemplates() {
	templates := []struct {
		template, name, description string
	}{
		{"gitcontribute://repository/{owner}/{repo}", "Repository", "Local repository record"},
		{"gitcontribute://thread/{owner}/{repo}/{kind}/{number}", "Thread", "Local issue or pull request"},
		{"gitcontribute://threads/{owner}/{repo}/{number}", "Numbered thread", "Local issue or pull request with kind resolved from the corpus"},
		{"gitcontribute://dossier/{owner}/{repo}", "Dossier", "Local source-backed repository dossier"},
		{"gitcontribute://investigation/{id}", "Investigation", "Local investigation workspace"},
		{"gitcontribute://opportunities/{investigation_id}", "Opportunities", "Local opportunities for an investigation"},
		{"gitcontribute://opportunity/{id}", "Opportunity", "Local contribution opportunity"},
		{"gitcontribute://evidence/{scope}/{id}", "Evidence", "Local evidence for an investigation or opportunity"},
		{"gitcontribute://readiness/{opportunity_id}", "Readiness", "Local contribution readiness report"},
		{"gitcontribute://workflow/contribution/{opportunity_id}", "Contribution workflow", "Safe contribution workflow resource links and prompts"},
		{"gitcontribute://lens/{name}", "Lens", "Saved lens definition"},
	}
	addResourceTemplates(s, templates)
}

func addResourceTemplates(s *Server, templates []struct {
	template, name, description string
}) {
	for _, t := range templates {
		s.server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: t.template,
			Name:        t.name,
			Description: t.description,
			MIMEType:    "application/json",
		}, s.readResource)
	}
}
