package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpShutdownGrace = 500 * time.Millisecond

// MCPStdioRunner uses the official MCP SDK to measure declared protocol
// milestones. It does not parse or infer protocol state from process output.
type MCPStdioRunner struct{}

// NewMCPStdioRunner returns an SDK-backed stdio validation runner.
func NewMCPStdioRunner() *MCPStdioRunner { return &MCPStdioRunner{} }

type connectedTransport struct{ connection mcp.Connection }

func (t connectedTransport) Connect(context.Context) (mcp.Connection, error) {
	return t.connection, nil
}

// Run starts an MCP stdio server, initializes a client session, lists tools,
// and closes the session while recording each protocol boundary.
func (r *MCPStdioRunner) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if len(req.Args) == 0 {
		return nil, ErrMissingCommand
	}
	if req.Dir == "" {
		return nil, ErrMissingWorkspace
	}
	info, err := os.Stat(req.Dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrInvalidWorkspace, req.Dir)
	}
	outputLimit := req.MaxOutputBytes
	if outputLimit < 0 || outputLimit > maxOutputBytes {
		return nil, ErrInvalidOutputLimit
	}
	if outputLimit == 0 {
		outputLimit = defaultMaxOutputBytes
	}
	readinessTimeout := req.ReadinessTimeout
	if readinessTimeout <= 0 {
		readinessTimeout = 30 * time.Second
	}

	started := time.Now().UTC()
	phases := RunPhases{SpawnStartedAt: started}
	stderr := newBoundedWriter(int(outputLimit))
	// #nosec G204 -- argv is a stored shell-free validation command whose execution requires explicit authorization.
	cmd := exec.CommandContext(ctx, req.Args[0], req.Args[1:]...)
	cmd.Dir = req.Dir
	if req.Env != nil {
		cmd.Env = req.Env
	}
	cmd.Stderr = stderr
	configureCommandCancellation(cmd)
	transport := &mcp.CommandTransport{Command: cmd, TerminateDuration: mcpShutdownGrace}
	connection, err := transport.Connect(ctx)
	if err != nil {
		return protocolStartResult(ctx, started, phases, stderr, err), nil
	}
	phases.ProcessStartedAt = time.Now().UTC()
	// #nosec G115 -- gopsutil models OS process IDs as int32 on supported platforms.
	sampler := startProcessSampler(ctx, int32(cmd.Process.Pid), req.SampleInterval)

	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-validation", Version: "v1"}, nil)
	session, readinessTimedOut, err := connectMCP(ctx, client, connection, cmd, readinessTimeout)
	if err != nil {
		var closer protocolCloser = connection
		if readinessTimedOut || ctx.Err() != nil {
			phases.ShutdownStartedAt = time.Now().UTC()
			closer = nil
		}
		return finishProtocolResult(ctx, cmd, sampler, started, phases, stderr, newBoundedWriter(int(outputLimit)), err, "readiness", readinessTimedOut, closer), nil
	}
	phases.InitializedAt = time.Now().UTC()
	phases.FirstResponseAt = phases.InitializedAt

	tools, listErr := session.ListTools(ctx, nil)
	if listErr == nil {
		phases.ToolsListedAt = time.Now().UTC()
	}
	stdout := newBoundedWriter(int(outputLimit))
	if encodeErr := json.NewEncoder(stdout).Encode(tools); encodeErr != nil && listErr == nil {
		listErr = fmt.Errorf("encode tools/list response: %w", encodeErr)
	}
	return finishProtocolResult(ctx, cmd, sampler, started, phases, stderr, stdout, listErr, "execution", errors.Is(ctx.Err(), context.DeadlineExceeded), session), nil
}

func protocolStartResult(ctx context.Context, started time.Time, phases RunPhases, stderr *boundedWriter, runErr error) *RunResult {
	completed := time.Now().UTC()
	timeoutPhase := ""
	classification := RunClassificationError
	if ctx.Err() != nil {
		classification = RunClassificationCancelled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			timeoutPhase = "startup"
		}
	}
	return &RunResult{
		ExitCode: -1, Stderr: stderr.String(), Truncated: stderr.Overflow(), StartedAt: started,
		CompletedAt: completed, Error: runErr.Error(), Classification: classification,
		Phases: phases, TimeoutPhase: timeoutPhase, FailurePhase: "startup",
		Cleanup: CleanupResult{Status: "unavailable", Reason: "process did not start", CheckedAt: completed},
	}
}

func finishProtocolResult(
	ctx context.Context,
	cmd *exec.Cmd,
	sampler *processSampler,
	started time.Time,
	phases RunPhases,
	stderr *boundedWriter,
	stdout *boundedWriter,
	runErr error,
	errorPhase string,
	timedOut bool,
	closer protocolCloser,
) *RunResult {
	phases.ExecutionEndedAt = time.Now().UTC()
	failurePhase := ""
	timeoutPhase := ""
	classification := RunClassificationPassing
	if runErr != nil {
		classification = RunClassificationError
		failurePhase = errorPhase
		if timedOut {
			timeoutPhase = errorPhase
		}
	}
	if phases.ShutdownStartedAt.IsZero() {
		phases.ShutdownStartedAt = time.Now().UTC()
	}
	var closeErr error
	shutdownTimedOut := false
	if closer != nil {
		shutdownTimedOut, closeErr = closeProtocol(ctx, closer, cmd)
	}
	if closeErr != nil && runErr == nil && !isExpectedMCPShutdownError(closeErr) {
		runErr = fmt.Errorf("close MCP session: %w", closeErr)
		classification = RunClassificationError
		failurePhase = "shutdown"
	}
	if shutdownTimedOut {
		failurePhase = "shutdown"
		timeoutPhase = "shutdown"
		classification = RunClassificationCancelled
	}
	sampled := sampler.finish()
	phases.ShutdownCheckedAt = sampled.cleanup.CheckedAt
	if ctx.Err() != nil && classification != RunClassificationPassing {
		classification = RunClassificationCancelled
	}
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	return &RunResult{
		ExitCode: exitCode(cmd), Stdout: stdout.String(), Stderr: stderr.String(),
		Truncated: stdout.Overflow() || stderr.Overflow(), StartedAt: started, CompletedAt: time.Now().UTC(),
		Error: errText, Classification: classification, Process: sampled.identity, Phases: phases,
		TimeoutPhase: timeoutPhase, FailurePhase: failurePhase,
		Resources: sampled.telemetry, Cleanup: sampled.cleanup,
	}
}

func isExpectedMCPShutdownError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ProcessState != nil && exitErr.ProcessState.ExitCode() == -1
}

type protocolCloser interface {
	Close() error
}

type mcpConnectResult struct {
	session *mcp.ClientSession
	err     error
}

func connectMCP(ctx context.Context, client *mcp.Client, connection mcp.Connection, cmd *exec.Cmd, timeout time.Duration) (*mcp.ClientSession, bool, error) {
	connected := make(chan mcpConnectResult, 1)
	go func() {
		session, err := client.Connect(ctx, connectedTransport{connection: connection}, nil)
		connected <- mcpConnectResult{session: session, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-connected:
		return result.session, false, result.err
	case <-timer.C:
		return nil, true, abortMCPConnect(connection, cmd, connected, context.DeadlineExceeded)
	case <-ctx.Done():
		return nil, errors.Is(ctx.Err(), context.DeadlineExceeded), abortMCPConnect(connection, cmd, connected, ctx.Err())
	}
}

func abortMCPConnect(connection mcp.Connection, cmd *exec.Cmd, connected <-chan mcpConnectResult, cause error) error {
	var cancelErr error
	if cmd.Cancel != nil {
		cancelErr = cmd.Cancel()
	}
	closed := make(chan error, 1)
	go func() { closed <- connection.Close() }()
	timer := time.NewTimer(mcpShutdownGrace)
	defer timer.Stop()
	var connectErr, closeErr error
	for remaining := 2; remaining > 0; {
		select {
		case result := <-connected:
			connectErr = result.err
			remaining--
			connected = nil
		case closeErr = <-closed:
			remaining--
			closed = nil
		case <-timer.C:
			return errors.Join(cause, cancelErr, connectErr, closeErr, errors.New("MCP readiness shutdown exceeded its grace period"))
		}
	}
	return errors.Join(cause, cancelErr, connectErr, closeErr)
}

func closeProtocol(ctx context.Context, closer protocolCloser, cmd *exec.Cmd) (bool, error) {
	done := make(chan error, 1)
	go func() { done <- closer.Close() }()
	select {
	case err := <-done:
		return false, err
	case <-ctx.Done():
		cancelErr := os.ErrProcessDone
		if cmd.Cancel != nil {
			cancelErr = cmd.Cancel()
		}
		timer := time.NewTimer(mcpShutdownGrace)
		defer timer.Stop()
		select {
		case closeErr := <-done:
			return true, errors.Join(ctx.Err(), cancelErr, closeErr)
		case <-timer.C:
			return true, errors.Join(ctx.Err(), cancelErr, errors.New("MCP shutdown exceeded its grace period"))
		}
	}
}

func exitCode(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}
