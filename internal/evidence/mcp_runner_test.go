package evidence

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type blockingProtocolCloser struct{ done <-chan struct{} }

func (c blockingProtocolCloser) Close() error {
	<-c.done
	return nil
}

func TestMCPStdioHelper(_ *testing.T) {
	if os.Getenv("GITCONTRIBUTE_MCP_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "validation-fixture", Version: "v1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "fixture.echo", Description: "fixture tool"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestMCPStdioRunnerRecordsSDKMilestones(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := NewMCPStdioRunner().Run(ctx, RunRequest{
		Args: []string{os.Args[0], "-test.run=^TestMCPStdioHelper$"}, Dir: t.TempDir(),
		Env: []string{"GITCONTRIBUTE_MCP_HELPER=1"}, MaxOutputBytes: 4096,
		ReadinessTimeout: time.Second, SampleInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Classification != RunClassificationPassing || result.FailurePhase != "" || result.TimeoutPhase != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Phases.InitializedAt.IsZero() || result.Phases.ToolsListedAt.IsZero() || result.Phases.ShutdownStartedAt.IsZero() {
		t.Fatalf("phases = %#v", result.Phases)
	}
	if !strings.Contains(result.Stdout, "fixture.echo") {
		t.Fatalf("tools/list output = %q", result.Stdout)
	}
}

func TestMCPStdioRunnerClassifiesReadinessDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := NewMCPStdioRunner().Run(ctx, RunRequest{
		Args: []string{"sh", "-c", "sleep 10"}, Dir: t.TempDir(),
		MaxOutputBytes: 1024, ReadinessTimeout: 20 * time.Millisecond,
		SampleInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TimeoutPhase != "readiness" || result.FailurePhase != "readiness" {
		t.Fatalf("phase result = %#v", result)
	}
	if result.Phases.ProcessStartedAt.IsZero() || !result.Phases.InitializedAt.IsZero() {
		t.Fatalf("phases = %#v", result.Phases)
	}
}

func TestCloseProtocolBoundsStuckShutdownAndUsesConfiguredCancellation(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelCalled := false
	cmd := &exec.Cmd{Cancel: func() error { cancelCalled = true; return nil }}
	started := time.Now()
	timedOut, err := closeProtocol(ctx, blockingProtocolCloser{done: done}, cmd)
	if !timedOut || err == nil || !cancelCalled {
		t.Fatalf("close result = timedOut:%v err:%v cancelCalled:%v", timedOut, err, cancelCalled)
	}
	if elapsed := time.Since(started); elapsed > 2*mcpShutdownGrace {
		t.Fatalf("shutdown took %s", elapsed)
	}
}
