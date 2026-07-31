package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type fakeDeepWikiReader struct {
	response deepwiki.Response
	request  deepwiki.Request
	calls    int
}

func (f *fakeDeepWikiReader) Read(_ context.Context, request deepwiki.Request) (deepwiki.Response, error) {
	f.calls++
	f.request = request
	return f.response, nil
}

func TestDeepWikiReturnsDerivedProvenanceAndBoundsOutput(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{Available: true, Text: strings.Repeat("x", 2048), SourceURL: "https://deepwiki.com/acme/rocket"}}
	svc.SetDeepWikiReader(fake)
	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpcontract.DeepWikiInput{Action: "question", Repositories: []string{"acme/rocket"}, Question: "architecture?", MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if out.Provenance != "derived_external" || !out.Truncated || len(out.Result) != 1024 {
		t.Fatalf("unexpected DeepWiki result: %+v", out)
	}
}

func TestDeepWikiUsesBoundedDefaultAndSteersFocusedRecovery(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{
		Available: true,
		Text:      strings.Repeat("x", mcpcontract.DeepWikiDefaultOutputBytes+1),
		SourceURL: "https://deepwiki.com/acme/rocket",
	}}
	svc.SetDeepWikiReader(fake)

	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpcontract.DeepWikiInput{Action: "contents", Repository: "acme/rocket"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Result) != mcpcontract.DeepWikiDefaultOutputBytes || !out.Truncated {
		t.Fatalf("default DeepWiki bound = %d bytes, truncated=%v", len(out.Result), out.Truncated)
	}
	if out.Reason != "output_limit" || out.Recovery == nil || len(out.Recovery.Then) != 1 || out.Recovery.Then[0].Type != "query_deepwiki" {
		t.Fatalf("missing truncation recovery guidance: %+v", out)
	}
}

func TestDeepWikiUsesNormalizedRepositoriesForRequestAndOutput(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{Available: true, Text: "ok"}}
	svc.SetDeepWikiReader(fake)
	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpcontract.DeepWikiInput{
		Action: "question", Repository: "acme/rocket", Repositories: []string{"wrong/one", "wrong/two"}, Question: "architecture?", MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"acme/rocket"}
	if !reflect.DeepEqual(fake.request.Repositories, want) || !reflect.DeepEqual(out.Repositories, want) {
		t.Fatalf("request repositories = %v, output repositories = %v", fake.request.Repositories, out.Repositories)
	}
}

func TestDeepWikiTruncationPreservesUTF8(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{Available: true, Text: strings.Repeat("x", 1023) + "€", SourceURL: "https://deepwiki.com/acme/rocket"}}
	svc.SetDeepWikiReader(fake)
	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpcontract.DeepWikiInput{Action: "contents", Repository: "acme/rocket", MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || len(out.Result) > 1024 || !utf8.ValidString(out.Result) {
		t.Fatalf("invalid bounded UTF-8 result: bytes=%d valid=%v truncated=%v", len(out.Result), utf8.ValidString(out.Result), out.Truncated)
	}
}

func TestDeepWikiRejectsOutputBoundsBeforeProviderRead(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	deepWiki := &fakeDeepWikiReader{}
	svc.SetDeepWikiReader(deepWiki)
	if _, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpcontract.DeepWikiInput{Action: "question", Repositories: []string{"acme/rocket"}, Question: "architecture?", MaxOutputBytes: 100}); err == nil {
		t.Fatal("DeepWiki accepted max_output_bytes below schema minimum")
	}
	if deepWiki.calls != 0 {
		t.Fatalf("DeepWiki provider called %d times for invalid input", deepWiki.calls)
	}
}
