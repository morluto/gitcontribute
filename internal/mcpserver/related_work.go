package mcpserver

// OpportunityRelatedWorkOutput is the compact MCP view of one Radar
// relationship. Exact source evidence remains available in the CLI JSON view.
type OpportunityRelatedWorkOutput struct {
	Ref       string `json:"ref"`
	Relation  string `json:"relation"`
	Direction string `json:"direction,omitempty"`
	State     string `json:"state,omitempty"`
}
