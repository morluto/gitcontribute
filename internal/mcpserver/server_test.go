package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeReader struct {
	searchStarted chan struct{}
}

func (f *fakeReader) Search(ctx context.Context, in SearchInput) (SearchOutput, error) {
	if in.Query == "block" {
		close(f.searchStarted)
		<-ctx.Done()
		return SearchOutput{}, ctx.Err()
	}
	match := ThreadOutput{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 7, State: "open", Title: "engine stalls"}
	return SearchOutput{Query: in.Query, Matches: []ThreadOutput{match}, Total: 1}, nil
}

func (*fakeReader) Repository(context.Context, RepoInput) (RepositoryOutput, error) {
	return RepositoryOutput{Owner: "acme", Repo: "rocket"}, nil
}

func (*fakeReader) Thread(_ context.Context, in ThreadInput) (ThreadOutput, error) {
	if in.Number == 404 {
		return ThreadOutput{}, ErrNotFound
	}
	return ThreadOutput{Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number, Title: "engine stalls"}, nil
}

func (*fakeReader) Dossier(context.Context, RepoInput) (DossierOutput, error) {
	return DossierOutput{Owner: "acme", Repo: "rocket", Sections: map[string]any{"open_issues": float64(1)}}, nil
}

func connect(t *testing.T, reader Reader) (*mcp.ClientSession, func()) {
	t.Helper()
	server := New(reader, "test")
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("connect client: %v", err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func TestToolsAreReadOnlyAndReturnStructuredOutput(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tools := map[string]*mcp.Tool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{"search", "get_repository", "get_thread", "get_dossier"} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", name, tool.Annotations)
		}
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search", Arguments: map[string]any{"query": "stall"},
	})
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if result.IsError {
		t.Fatalf("search returned tool error: %+v", result.Content)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out SearchOutput
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if out.Total != 1 || len(out.Matches) != 1 || out.Matches[0].Number != 7 {
		t.Fatalf("search output = %+v", out)
	}
}

func TestRepositoryResourceAndNotFound(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://repository/acme/rocket",
	})
	if err != nil {
		t.Fatalf("read repository: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text == "" {
		t.Fatalf("resource result = %+v", result)
	}

	_, err = client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://thread/acme/rocket/issue/404",
	})
	if err == nil {
		t.Fatal("expected resource-not-found error")
	}
}

func TestToolCancellationReachesReader(t *testing.T) {
	fake := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, fake)
	defer closeSessions()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.CallTool(ctx, &mcp.CallToolParams{
			Name: "search", Arguments: map[string]any{"query": "block"},
		})
		done <- err
	}()
	<-fake.searchStarted
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("call error = %v, want context canceled", err)
	}
}
