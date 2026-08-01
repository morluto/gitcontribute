package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const testArtifactDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type acquisitionArtifactReader struct {
	*fakeReader
}

type acquisitionCapabilityReader struct {
	*acquisitionArtifactReader
}

func (*acquisitionCapabilityReader) SearchGitHubThreads(context.Context, mcpcontract.SearchGitHubThreadsInput) (mcpcontract.SearchGitHubThreadsOutput, error) {
	return mcpcontract.SearchGitHubThreadsOutput{ResourceURI: "gitcontribute://artifact/github-thread-search/" + testArtifactDigest}, nil
}

func (*acquisitionCapabilityReader) ReadSourceFiles(context.Context, mcpcontract.ReadSourceFilesInput) (mcpcontract.ReadSourceFilesOutput, error) {
	return mcpcontract.ReadSourceFilesOutput{ResourceURI: "gitcontribute://artifact/source-bundle/" + testArtifactDigest}, nil
}

func (*acquisitionCapabilityReader) SearchCodeBatch(context.Context, mcpcontract.SearchCodeBatchInput) (mcpcontract.SearchCodeBatchOutput, error) {
	return mcpcontract.SearchCodeBatchOutput{Status: "complete"}, nil
}

func (r *acquisitionArtifactReader) ReadGitHubThreadSearchArtifact(context.Context, string) (mcpcontract.GitHubThreadSearchArtifact, error) {
	return mcpcontract.GitHubThreadSearchArtifact{SchemaVersion: "github-thread-search.v1", ArtifactKind: "github-thread-search.v1", Query: "needle"}, nil
}

func (r *acquisitionArtifactReader) ReadSourceBundleArtifact(context.Context, string) (mcpcontract.SourceBundleArtifact, error) {
	return mcpcontract.SourceBundleArtifact{SchemaVersion: "source-bundle.v1", ArtifactKind: "source-bundle.v1", CommitSHA: "commit-sha"}, nil
}

func TestAcquisitionArtifactResourcesRouteExactOpaqueURIs(t *testing.T) {
	reader := &acquisitionArtifactReader{fakeReader: &fakeReader{searchStarted: make(chan struct{})}}
	server := &Server{reader: reader}
	for _, test := range []struct {
		uri    string
		assert func(t *testing.T, value any)
	}{
		{
			uri: "gitcontribute://artifact/github-thread-search/" + testArtifactDigest,
			assert: func(t *testing.T, value any) {
				artifact, ok := value.(mcpcontract.GitHubThreadSearchArtifact)
				if !ok || artifact.ArtifactKind != "github-thread-search.v1" || artifact.Query != "needle" {
					t.Fatalf("thread artifact = %#v", value)
				}
			},
		},
		{
			uri: "gitcontribute://artifact/source-bundle/" + testArtifactDigest,
			assert: func(t *testing.T, value any) {
				artifact, ok := value.(mcpcontract.SourceBundleArtifact)
				if !ok || artifact.ArtifactKind != "source-bundle.v1" || artifact.CommitSHA != "commit-sha" {
					t.Fatalf("source artifact = %#v", value)
				}
			},
		},
	} {
		u := strings.TrimPrefix(test.uri, "gitcontribute://")
		host, path, _ := strings.Cut(u, "/")
		value, err := server.readResourceValue(context.Background(), resourceRequest{uri: test.uri, scheme: "gitcontribute", host: host, parts: strings.Split(path, "/")})
		if err != nil {
			t.Fatalf("read %s: %v", test.uri, err)
		}
		test.assert(t, value)
	}
	if _, err := server.readResourceValue(context.Background(), resourceRequest{
		uri: "gitcontribute://artifact/unknown/" + testArtifactDigest, scheme: "gitcontribute", host: "artifact", parts: []string{"unknown", testArtifactDigest},
	}); err == nil {
		t.Fatal("unknown artifact namespace was routed")
	}
}

func TestAcquisitionArtifactResourceTemplatesTrackReaderCapabilities(t *testing.T) {
	reader := &acquisitionArtifactReader{fakeReader: &fakeReader{searchStarted: make(chan struct{})}}
	client, closeSessions := connect(t, reader)
	defer closeSessions()

	want := map[string]bool{
		"gitcontribute://artifact/github-thread-search/{artifact_digest}": false,
		"gitcontribute://artifact/source-bundle/{artifact_digest}":        false,
	}
	for template, err := range client.ResourceTemplates(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := want[template.URITemplate]; ok {
			want[template.URITemplate] = true
		}
	}
	for template, found := range want {
		if !found {
			t.Errorf("missing resource template %q", template)
		}
	}
}

func TestAcquisitionToolsExposeTheirSideEffectBoundaries(t *testing.T) {
	reader := &acquisitionCapabilityReader{acquisitionArtifactReader: &acquisitionArtifactReader{fakeReader: &fakeReader{searchStarted: make(chan struct{})}}}
	client, closeSessions := connect(t, reader)
	defer closeSessions()

	tools := map[string]bool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = true
		if tool.Name == mcpcontract.ToolSearchGitHubThreads || tool.Name == mcpcontract.ToolReadSourceFiles {
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
				t.Errorf("live acquisition annotations for %s = %+v", tool.Name, tool.Annotations)
			}
		}
	}
	for _, name := range []string{mcpcontract.ToolSearchGitHubThreads, mcpcontract.ToolReadSourceFiles, mcpcontract.ToolSearchCodeBatch} {
		if !tools[name] {
			t.Errorf("tools/list missing %s", name)
		}
	}
}
