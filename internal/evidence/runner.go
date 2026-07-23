package evidence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const defaultMaxOutputBytes = 64 * 1024
const maxOutputBytes = 64 * 1024 * 1024

// ExecRunner executes commands directly, without a shell, with bounded output capture.
type ExecRunner struct{}

// NewExecRunner returns a shell-free Runner backed by os/exec.
func NewExecRunner() *ExecRunner {
	return &ExecRunner{}
}

// Run starts the command described by req.Args inside req.Dir. It preserves
// context cancellation, captures stdout and stderr up to req.MaxOutputBytes per
// stream, records timing, and never invokes a shell.
func (r *ExecRunner) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
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

	max := req.MaxOutputBytes
	if max < 0 || max > maxOutputBytes {
		return nil, ErrInvalidOutputLimit
	}
	if max == 0 {
		max = defaultMaxOutputBytes
	}

	cmd := exec.CommandContext(ctx, req.Args[0], req.Args[1:]...)
	cmd.Dir = req.Dir
	if req.Env != nil {
		cmd.Env = req.Env
	}
	stdoutBuf := newBoundedWriter(int(max))
	stderrBuf := newBoundedWriter(int(max))
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	configureCommandCancellation(cmd)
	cmd.WaitDelay = 2 * time.Second

	started := time.Now().UTC()
	phases := RunPhases{SpawnStartedAt: started}
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			completed := time.Now().UTC()
			timeoutPhase := ""
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				timeoutPhase = "startup"
			}
			return &RunResult{
				ExitCode:       -1,
				StartedAt:      started,
				CompletedAt:    completed,
				Error:          ctx.Err().Error(),
				Classification: RunClassificationCancelled,
				Phases:         phases,
				TimeoutPhase:   timeoutPhase,
				FailurePhase:   "startup",
				Cleanup:        CleanupResult{Status: "unavailable", Reason: "process did not start", CheckedAt: completed},
			}, nil
		}
		completed := time.Now().UTC()
		return &RunResult{
			ExitCode: -1, StartedAt: started, CompletedAt: completed,
			Error: fmt.Sprintf("runner: start: %v", err), Classification: RunClassificationError,
			Phases: phases, FailurePhase: "startup",
			Cleanup: CleanupResult{Status: "unavailable", Reason: "process did not start", CheckedAt: completed},
		}, nil
	}
	phases.ProcessStartedAt = time.Now().UTC()
	// #nosec G115 -- gopsutil models OS process IDs as int32 on supported platforms.
	sampler := startProcessSampler(ctx, int32(cmd.Process.Pid), req.SampleInterval)

	runErr := cmd.Wait()
	completed := time.Now().UTC()
	phases.ExecutionEndedAt = completed
	phases.ShutdownStartedAt = completed
	sampled := sampler.finish()
	phases.ShutdownCheckedAt = sampled.cleanup.CheckedAt

	if ctx.Err() != nil {
		timeoutPhase := ""
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			timeoutPhase = "execution"
		}
		return &RunResult{
			ExitCode:       -1,
			Stdout:         stdoutBuf.String(),
			Stderr:         stderrBuf.String(),
			Truncated:      stdoutBuf.Overflow() || stderrBuf.Overflow(),
			StartedAt:      started,
			CompletedAt:    completed,
			Error:          ctx.Err().Error(),
			Classification: RunClassificationCancelled,
			Process:        sampled.identity,
			Phases:         phases,
			TimeoutPhase:   timeoutPhase,
			FailurePhase:   "execution",
			Resources:      sampled.telemetry,
			Cleanup:        sampled.cleanup,
		}, nil
	}

	exitCode := 0
	runErrStr := ""
	classification := RunClassificationPassing
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			if exitCode != 0 {
				classification = RunClassificationFailing
			}
		} else {
			exitCode = -1
			classification = RunClassificationError
		}
		runErrStr = runErr.Error()
	}
	timeoutPhase := ""
	if errors.Is(runErr, exec.ErrWaitDelay) {
		timeoutPhase = "shutdown"
	}
	failurePhase := ""
	if runErr != nil {
		failurePhase = "execution"
	}
	if timeoutPhase == "shutdown" {
		failurePhase = "shutdown"
	}

	return &RunResult{
		ExitCode:       exitCode,
		Stdout:         stdoutBuf.String(),
		Stderr:         stderrBuf.String(),
		Truncated:      stdoutBuf.Overflow() || stderrBuf.Overflow(),
		StartedAt:      started,
		CompletedAt:    completed,
		Error:          runErrStr,
		Classification: classification,
		Process:        sampled.identity,
		Phases:         phases,
		TimeoutPhase:   timeoutPhase,
		FailurePhase:   failurePhase,
		Resources:      sampled.telemetry,
		Cleanup:        sampled.cleanup,
	}, nil
}

// boundedWriter captures up to a byte limit and records whether content was discarded.
type boundedWriter struct {
	cap      int
	buf      []byte
	overflow bool
}

func newBoundedWriter(cap int) *boundedWriter {
	if cap < 0 {
		cap = 0
	}
	return &boundedWriter{cap: cap, buf: make([]byte, 0, cap)}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if w.cap <= 0 {
		if n > 0 {
			w.overflow = true
		}
		return n, nil
	}
	if len(w.buf) < w.cap {
		space := w.cap - len(w.buf)
		if space > len(p) {
			space = len(p)
		}
		w.buf = append(w.buf, p[:space]...)
		p = p[space:]
	}
	if len(p) > 0 {
		w.overflow = true
	}
	return n, nil
}

func (w *boundedWriter) String() string { return string(w.buf) }
func (w *boundedWriter) Overflow() bool { return w.overflow }
