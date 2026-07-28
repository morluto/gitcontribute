package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// linkedResource keeps structured tool output under the SDK's generic AddTool
// path while giving MCP clients a native, followable durable-object link.
func linkedResource(uri, name, title, description string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ResourceLink{
		URI: uri, Name: name, Title: title, Description: description, MIMEType: "application/json",
	}}}
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
			content = append(content, &mcp.ResourceLink{
				URI: artifact.URI, Name: artifact.Kind, Title: "Job artifact",
				Description: "Durable typed artifact produced by " + item.Value.Kind + ".", MIMEType: "application/json",
			})
		}
	}
	if len(content) == 0 {
		return nil
	}
	return &mcp.CallToolResult{Content: content}
}
