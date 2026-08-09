package deepwiki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClientReadRoutesRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, action, repository, question, wantName string
		repositories                                 []string
		wantArgs                                     map[string]any
	}{
		{name: "structure", action: "structure", repository: "owner/repo", wantName: "read_wiki_structure", wantArgs: map[string]any{"repoName": "owner/repo"}},
		{name: "contents", action: "contents", repository: "owner/repo", wantName: "read_wiki_contents", wantArgs: map[string]any{"repoName": "owner/repo"}},
		{name: "single question", action: "question", repositories: []string{"owner/repo"}, question: "How?", wantName: "ask_question", wantArgs: map[string]any{"repoName": "owner/repo", "question": "How?"}},
		{name: "multi question", action: "question", repositories: []string{"one/repo", "two/repo"}, question: "Compare", wantName: "ask_question", wantArgs: map[string]any{"repoName": []string{"one/repo", "two/repo"}, "question": "Compare"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mu   sync.Mutex
				name string
				args map[string]any
			)
			client := newTestClient(t, func(gotName string, gotArgs map[string]any) (*mcp.CallToolResult, error) {
				mu.Lock()
				defer mu.Unlock()
				name, args = gotName, gotArgs
				return &mcp.CallToolResult{}, nil
			})
			if _, err := client.Read(context.Background(), Request{Action: tt.action, Repository: tt.repository, Repositories: tt.repositories, Question: tt.question}); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			gotArgs, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			wantArgs, err := json.Marshal(tt.wantArgs)
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName || string(gotArgs) != string(wantArgs) {
				t.Fatalf("tool call = %q, %#v; want %q, %#v", name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestClientReadRejectsMissingAndUnsupportedInputs(t *testing.T) {
	t.Parallel()
	for _, req := range []Request{{Action: "structure"}, {Action: "contents"}, {Action: "question"}, {Action: "unknown"}} {
		if _, err := (&Client{}).Read(context.Background(), req); err == nil {
			t.Fatalf("Read(%+v) accepted invalid input", req)
		}
	}
}

func TestClientReadMapsResponse(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(name string, args map[string]any) (*mcp.CallToolResult, error) {
		if name != "read_wiki_contents" || args["repoName"] != "owner/repo" {
			return nil, &unexpectedToolCallError{name: name, args: args}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "first"}, &mcp.TextContent{Text: "https://deepwiki.com/owner/repo#topic"}}}, nil
	})
	got, err := client.Read(context.Background(), Request{Action: "contents", Repository: "owner/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Text != "first\nhttps://deepwiki.com/owner/repo#topic" || got.SourceURL != "https://deepwiki.com/owner/repo#topic" {
		t.Fatalf("response = %+v", got)
	}
}

func TestClientReadHandlesProviderAndTransportFailures(t *testing.T) {
	t.Parallel()
	provider := newTestClient(t, func(string, map[string]any) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	})
	got, err := provider.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err != nil || got.Available {
		t.Fatalf("provider error = %+v, %v", got, err)
	}

	transportServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(transportServer.Close)
	transport := &Client{Endpoint: transportServer.URL}
	_, err = transport.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err == nil || !strings.Contains(err.Error(), "call DeepWiki read_wiki_structure:") {
		t.Fatalf("transport error = %v", err)
	}
}

func TestClientReadClassifiesProviderErrorTextAsUnavailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
	}{
		{
			name: "question with mixed repository availability",
			text: "Error processing question: Repository not found. Visit https://deepwiki.com to index it. Requested repos: indexed/repo, missing/repo",
		},
		{
			name: "structure unavailable",
			text: "Error fetching wiki structure: Repository not found.",
		},
		{
			name: "bare repository error",
			text: "Repository not found. Visit https://deepwiki.com to index it.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(string, map[string]any) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: tt.text}}}, nil
			})
			got, err := client.Read(context.Background(), Request{
				Action:       "question",
				Repositories: []string{"indexed/repo", "missing/repo"},
				Question:     "Compare them.",
			})
			if err != nil || got.Available || got.Text != tt.text {
				t.Fatalf("provider error text = %+v, %v", got, err)
			}
		})
	}
}

func TestClientReadKeepsNormalMultiRepositoryAnswerAvailable(t *testing.T) {
	t.Parallel()
	const answer = "indexed/repo and other/repo both organize documentation by subsystem."
	client := newTestClient(t, func(string, map[string]any) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: answer}}}, nil
	})
	got, err := client.Read(context.Background(), Request{
		Action:       "question",
		Repositories: []string{"indexed/repo", "other/repo"},
		Question:     "Compare them.",
	})
	if err != nil || !got.Available || got.Text != answer {
		t.Fatalf("normal answer = %+v, %v", got, err)
	}
}

func TestClientReadAcceptsEmptySuccessfulResponse(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(string, map[string]any) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	got, err := client.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err != nil || !got.Available || got.Text != "" || got.SourceURL != "" {
		t.Fatalf("empty response = %+v, %v", got, err)
	}
}

type unexpectedToolCallError struct {
	name string
	args map[string]any
}

func (e *unexpectedToolCallError) Error() string {
	return "unexpected DeepWiki tool call " + e.name
}

func newTestClient(t *testing.T, respond func(string, map[string]any) (*mcp.CallToolResult, error)) *Client {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "deepwiki-fixture", Version: "1"}, nil)
	for _, name := range []string{"read_wiki_structure", "read_wiki_contents", "ask_question"} {
		mcp.AddTool(server, &mcp.Tool{Name: name}, func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			result, err := respond(name, args)
			return result, nil, err
		})
	}
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	t.Cleanup(httpServer.Close)
	return &Client{Endpoint: httpServer.URL}
}
