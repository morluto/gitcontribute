package mcpcontract

const CatalogContractVersion = "gitcontribute.catalog.v1"

// GetCatalogContractInput is intentionally empty: the contract describes the
// catalog exposed by the current MCP server instance.
type GetCatalogContractInput struct{}

// CatalogRoute describes one canonical workflow route and whether each
// capability survived runtime registration and read-only filtering.
type CatalogRoute struct {
	IndexTool                 string   `json:"index_tool"`
	SyncTool                  string   `json:"sync_tool"`
	SearchTool                string   `json:"search_tool"`
	IndexAdvertised           bool     `json:"index_advertised"`
	SyncAdvertised            bool     `json:"sync_advertised"`
	SearchAdvertised          bool     `json:"search_advertised"`
	CanonicalAsyncChain       []string `json:"canonical_async_chain"`
	GenericTextSearchBoundary string   `json:"generic_text_search_boundary"`
}

// CatalogContract is a compact, model-visible parity record for the actual
// post-registration MCP catalog. It helps clients distinguish an old or
// restricted server from an implementation that lacks a capability.
type CatalogContract struct {
	SchemaVersion       string       `json:"schema_version"`
	ServerName          string       `json:"server_name"`
	ServerVersion       string       `json:"server_version"`
	CatalogMode         string       `json:"catalog_mode"`
	CatalogFingerprint  string       `json:"catalog_fingerprint"`
	ToolCount           int          `json:"tool_count"`
	PullRequestFeedback CatalogRoute `json:"pull_request_feedback"`
}
