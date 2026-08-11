package mcpserver

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

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

	for _, uri := range []string{
		"gitcontribute://repository/acme/rocket?view=full",
		"gitcontribute://repository/acme/rocket#fragment",
		"gitcontribute://user@repository/acme/rocket",
		"gitcontribute://repository/acme/rocket/",
		"gitcontribute://repository/acme//rocket",
	} {
		if _, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatalf("non-canonical resource URI %q was accepted", uri)
		}
	}
}

type feedbackResourceCapture struct {
	*fakeReader
	channel    string
	feedbackID string
}

func (*feedbackResourceCapture) PullRequestFeedbackResource(context.Context, string, string, int) (mcpcontract.PullRequestFeedbackResource, error) {
	return mcpcontract.PullRequestFeedbackResource{}, errors.New("unexpected pull-request feedback resource")
}

func (r *feedbackResourceCapture) PullRequestFeedbackItemResource(_ context.Context, _ string, _ string, _ int, channel, feedbackID string) (mcpcontract.PullRequestFeedbackItemResource, error) {
	r.channel = channel
	r.feedbackID = feedbackID
	return mcpcontract.PullRequestFeedbackItemResource{SchemaVersion: "gitcontribute.pull-request-feedback-item.v1"}, nil
}

func (*feedbackResourceCapture) CIFailureResource(context.Context, string, string, int) (mcpcontract.CIFailureResource, error) {
	return mcpcontract.CIFailureResource{}, errors.New("unexpected CI failure resource")
}

func (*feedbackResourceCapture) CIJobLogResource(context.Context, string, string, int, int64) (mcpcontract.CIJobLogResource, error) {
	return mcpcontract.CIJobLogResource{}, errors.New("unexpected CI job log resource")
}

func TestResourcePathPartsPreservesEscapedOpaqueIDs(t *testing.T) {
	parts, ok := resourcePathParts("/acme/rocket/7/inline_comments/comment%2Fwith%20space")
	if !ok {
		t.Fatal("resourcePathParts rejected a valid escaped path")
	}
	want := []string{"acme", "rocket", "7", "inline_comments", "comment/with space"}
	if !slices.Equal(parts, want) {
		t.Fatalf("resource path parts = %q, want %q", parts, want)
	}
}

func TestResourcePathPartsRejectsMalformedEscapes(t *testing.T) {
	if _, ok := resourcePathParts("/acme/rocket/%zz"); ok {
		t.Fatal("resourcePathParts accepted a malformed escape")
	}
	server := &Server{reader: &fakeReader{}}
	if _, err := server.readResource(context.Background(), &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitcontribute://repository/acme/%zz"}}); err == nil {
		t.Fatal("malformed resource URI was accepted")
	}
}

func TestReadResourceRoutesEscapedFeedbackIDAsOneOpaqueSegment(t *testing.T) {
	reader := &feedbackResourceCapture{fakeReader: &fakeReader{}}
	server := &Server{reader: reader}
	result, err := server.readResource(context.Background(), &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitcontribute://pull-request-feedback/acme/rocket/7/inline_comments/comment%2Fwith%20space"}})
	if err != nil {
		t.Fatalf("read escaped feedback resource: %v", err)
	}
	if len(result.Contents) != 1 || reader.channel != "inline_comments" || reader.feedbackID != "comment/with space" {
		t.Fatalf("escaped feedback resource routed as channel=%q feedback_id=%q result=%+v", reader.channel, reader.feedbackID, result)
	}
}

func TestSearchCodeRejectsWhitespaceOnlyQuery(t *testing.T) {
	server := &Server{reader: &fakeReader{}}
	_, _, err := server.searchCode(context.Background(), nil, mcpcontract.SearchCodeInput{Query: " \t "})
	if err == nil {
		t.Fatal("whitespace-only code search query was accepted")
	}
}

func TestSearchThreadsRejectsWhitespaceOnlyQuery(t *testing.T) {
	server := &Server{reader: &fakeReader{}}
	_, _, err := server.searchThreads(context.Background(), nil, mcpcontract.SearchInput{Query: " \t "})
	if err == nil {
		t.Fatal("whitespace-only thread search query was accepted")
	}
}

func TestSearchRepositoriesNormalizesOptionalQuery(t *testing.T) {
	server := &Server{reader: &fakeReader{}}
	_, out, err := server.searchRepositories(context.Background(), nil, mcpcontract.SearchRepositoriesInput{Query: " \t "})
	if err != nil {
		t.Fatalf("search repositories: %v", err)
	}
	if out.Query != "" {
		t.Fatalf("repository query was not normalized: %q", out.Query)
	}
}
