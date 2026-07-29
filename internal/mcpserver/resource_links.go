package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const resourceReadGuidance = " Pass this exact opaque URI unchanged to the host MCP resource reader; do not shorten, pluralize, or reconstruct it."

func resourceReadInstruction(uri string) *mcp.TextContent {
	return &mcp.TextContent{Text: fmt.Sprintf(
		"Ask the host to perform MCP `resources/read` with this server and the exact URI %q; in Codex, call `read_mcp_resource`. Treat this URI as opaque: copy it verbatim without shortening, pluralizing, or reconstructing it. Do not substitute structured tool output for the resource read.",
		uri,
	)}
}

// linkedResource keeps structured tool output under the SDK's generic AddTool
// path while giving MCP clients a native, followable durable-object link.
func linkedResource(uri, name, title, description string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{
		resourceReadInstruction(uri),
		&mcp.ResourceLink{
			URI: uri, Name: name, Title: title, Description: description + resourceReadGuidance, MIMEType: "application/json",
		},
	}}
}

func linkedJobResources(out mcpcontract.GetJobsOutput) *mcp.CallToolResult {
	var content []mcp.Content
	for _, item := range out.Items {
		if item.Value == nil {
			continue
		}
		for _, artifact := range item.Value.Artifacts {
			if artifact.URI == "" {
				continue
			}
			content = append(
				content,
				resourceReadInstruction(artifact.URI),
				&mcp.ResourceLink{
					URI: artifact.URI, Name: artifact.Kind, Title: "Job artifact",
					Description: "Durable typed artifact produced by " + item.Value.Kind + "." + resourceReadGuidance,
					MIMEType:    "application/json",
				},
			)
		}
	}
	if len(content) == 0 {
		return nil
	}
	return &mcp.CallToolResult{Content: content}
}
