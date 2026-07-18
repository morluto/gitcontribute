// Package deepwiki adapts the public DeepWiki MCP server behind a narrow,
// product-owned read contract. Returned prose is untrusted derived context.
package deepwiki

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultEndpoint is DeepWiki's unauthenticated public Streamable HTTP MCP endpoint.
const DefaultEndpoint = "https://mcp.deepwiki.com/mcp"

// Request selects one public DeepWiki read action. Structure and contents use
// Repository; question uses Repositories and Question.
type Request struct {
	Action       string
	Repository   string
	Repositories []string
	Question     string
}

// Response contains untrusted derived prose. Available is false for a
// provider-level tool error; transport and protocol failures are returned as errors.
type Response struct {
	Text      string
	SourceURL string
	Available bool
}

// Reader performs an external DeepWiki read without writing to the local corpus.
type Reader interface {
	Read(context.Context, Request) (Response, error)
}

// Client calls a public DeepWiki MCP endpoint. An empty Endpoint uses DefaultEndpoint.
type Client struct{ Endpoint string }

var sourceURLPattern = regexp.MustCompile(`https://deepwiki\.com/[^\s)\]}>]+`)

// Read performs one public DeepWiki tool call and returns text content only. It
// neither persists the response nor treats it as GitHub authority.
func (c *Client) Read(ctx context.Context, req Request) (Response, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	name, arguments, err := toolCall(req)
	if err != nil {
		return Response{}, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		return Response{}, fmt.Errorf("connect DeepWiki: %w", err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return Response{}, fmt.Errorf("call DeepWiki %s: %w", name, err)
	}
	if result.IsError {
		return Response{Available: false}, nil
	}
	var textParts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			textParts = append(textParts, text.Text)
		}
	}
	text := strings.Join(textParts, "\n")
	return Response{Text: text, SourceURL: sourceURLPattern.FindString(text), Available: true}, nil
}

func toolCall(req Request) (string, map[string]any, error) {
	switch req.Action {
	case "structure":
		if req.Repository == "" {
			return "", nil, errors.New("repository is required")
		}
		return "read_wiki_structure", map[string]any{"repoName": req.Repository}, nil
	case "contents":
		if req.Repository == "" {
			return "", nil, errors.New("repository is required")
		}
		return "read_wiki_contents", map[string]any{"repoName": req.Repository}, nil
	case "question":
		if len(req.Repositories) == 0 || req.Question == "" {
			return "", nil, errors.New("repositories and question are required")
		}
		var repoName any = req.Repositories
		if len(req.Repositories) == 1 {
			repoName = req.Repositories[0]
		}
		return "ask_question", map[string]any{"repoName": repoName, "question": req.Question}, nil
	default:
		return "", nil, fmt.Errorf("unsupported DeepWiki action %q", req.Action)
	}
}
