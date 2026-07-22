package deepwiki

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolCall(t *testing.T) {
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
			name, args, err := toolCall(Request{Action: tt.action, Repository: tt.repository, Repositories: tt.repositories, Question: tt.question})
			if err != nil || name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("toolCall = %q, %#v, %v; want %q, %#v", name, args, err, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestToolCallRejectsMissingAndUnsupportedInputs(t *testing.T) {
	t.Parallel()
	for _, req := range []Request{{Action: "structure"}, {Action: "contents"}, {Action: "question"}, {Action: "unknown"}} {
		if _, _, err := toolCall(req); err == nil {
			t.Fatalf("toolCall(%+v) accepted invalid input", req)
		}
	}
}

func TestClientReadMapsResponse(t *testing.T) {
	t.Parallel()
	client := &Client{callTool: func(_ context.Context, endpoint, name string, args map[string]any) (*mcp.CallToolResult, error) {
		if endpoint != DefaultEndpoint || name != "read_wiki_contents" || args["repoName"] != "owner/repo" {
			t.Fatalf("call = %q, %q, %#v", endpoint, name, args)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "first"}, &mcp.TextContent{Text: "https://deepwiki.com/owner/repo#topic"}}}, nil
	}}
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
	provider := &Client{callTool: func(context.Context, string, string, map[string]any) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	}}
	got, err := provider.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err != nil || got.Available {
		t.Fatalf("provider error = %+v, %v", got, err)
	}

	transport := &Client{callTool: func(context.Context, string, string, map[string]any) (*mcp.CallToolResult, error) {
		return nil, errors.New("offline")
	}}
	_, err = transport.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err == nil || !strings.Contains(err.Error(), "call DeepWiki read_wiki_structure: offline") {
		t.Fatalf("transport error = %v", err)
	}
}

func TestClientReadAcceptsEmptySuccessfulResponse(t *testing.T) {
	t.Parallel()
	client := &Client{callTool: func(context.Context, string, string, map[string]any) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}}
	got, err := client.Read(context.Background(), Request{Action: "structure", Repository: "owner/repo"})
	if err != nil || !got.Available || got.Text != "" || got.SourceURL != "" {
		t.Fatalf("empty response = %+v, %v", got, err)
	}
}
