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

type responseState uint8

const (
	responseUnavailable responseState = iota
	responseAvailable
)

// Response contains untrusted derived prose. Its private state keeps provider
// unavailability distinct from a successful response; transport and protocol
// failures are returned as errors.
type Response struct {
	state     responseState
	text      string
	sourceURL string
}

func UnavailableResponse(text string) Response { return Response{text: text} }
func AvailableResponse(text, sourceURL string) Response {
	return Response{state: responseAvailable, text: text, sourceURL: sourceURL}
}
func (r Response) Available() bool   { return r.state == responseAvailable }
func (r Response) Text() string      { return r.text }
func (r Response) SourceURL() string { return r.sourceURL }

// Reader performs an external DeepWiki read without writing to the local corpus.
type Reader interface {
	Read(context.Context, Request) (Response, error)
}

// Client calls a public DeepWiki MCP endpoint. An empty Endpoint uses DefaultEndpoint.
type Client struct {
	Endpoint string
}

var (
	sourceURLPattern      = regexp.MustCompile(`https://deepwiki\.com/[^\s)\]}>]+`)
	providerErrorPrefixes = []string{
		"error processing question:",
		"error fetching wiki structure:",
		"error fetching wiki contents:",
	}
)

// Read performs one public DeepWiki tool call and returns text content only. It
// neither persists the response nor treats it as GitHub authority.
func (c *Client) Read(ctx context.Context, req Request) (_ Response, err error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	name, arguments, err := toolCall(req)
	if err != nil {
		return Response{}, err
	}
	result, err := callDeepWikiTool(ctx, endpoint, name, arguments)
	if err != nil {
		return Response{}, fmt.Errorf("call DeepWiki %s: %w", name, err)
	}
	if result.IsError {
		return UnavailableResponse(""), nil
	}
	var textParts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			textParts = append(textParts, text.Text)
		}
	}
	text := strings.Join(textParts, "\n")
	if isProviderErrorText(text) {
		return UnavailableResponse(text), nil
	}
	return AvailableResponse(text, sourceURLPattern.FindString(text)), nil
}

func isProviderErrorText(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range providerErrorPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return strings.HasPrefix(normalized, "repository not found.") &&
		strings.Contains(normalized, "deepwiki.com")
}

func callDeepWikiTool(ctx context.Context, endpoint, name string, arguments map[string]any) (_ *mcp.CallToolResult, err error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() {
		if closeErr := session.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close session: %w", closeErr)
		}
	}()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func toolCall(req Request) (string, map[string]any, error) {
	if req == nil {
		return "", nil, errors.New("DeepWiki request is required")
	}
	repositories := req.Repositories()
	switch req.Action() {
	case Structure:
		return "read_wiki_structure", map[string]any{"repoName": repositories[0]}, nil
	case Contents:
		return "read_wiki_contents", map[string]any{"repoName": repositories[0]}, nil
	case Question:
		var repoName any = repositories
		if len(repositories) == 1 {
			repoName = repositories[0]
		}
		return "ask_question", map[string]any{"repoName": repoName, "question": req.Question()}, nil
	default:
		return "", nil, errors.New("invalid parsed DeepWiki request")
	}
}
