package evidence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const defaultMaxOutputBytes = 64 * 1024

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
	if max <= 0 {
		max = defaultMaxOutputBytes
	}

	cmd := exec.CommandContext(ctx, req.Args[0], req.Args[1:]...)
	cmd.Dir = req.Dir
	if req.Env != nil {
		cmd.Env = req.Env
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("runner: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("runner: stderr pipe: %w", err)
	}

	stdoutBuf := newBoundedWriter(int(max))
	stderrBuf := newBoundedWriter(int(max))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		_, _ = io.Copy(stdoutBuf, stdoutPipe)
		wg.Done()
	}()
	go func() {
		_, _ = io.Copy(stderrBuf, stderrPipe)
		wg.Done()
	}()

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("runner: start: %w", err)
	}

	runErr := cmd.Wait()
	wg.Wait()
	completed := time.Now()

	if ctx.Err() != nil {
		return &RunResult{
			ExitCode:       -1,
			Stdout:         stdoutBuf.String(),
			Stderr:         stderrBuf.String(),
			Truncated:      stdoutBuf.Overflow() || stderrBuf.Overflow(),
			StartedAt:      started,
			CompletedAt:    completed,
			Error:          ctx.Err().Error(),
			Classification: RunClassificationCancelled,
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

	return &RunResult{
		ExitCode:       exitCode,
		Stdout:         stdoutBuf.String(),
		Stderr:         stderrBuf.String(),
		Truncated:      stdoutBuf.Overflow() || stderrBuf.Overflow(),
		StartedAt:      started,
		CompletedAt:    completed,
		Error:          runErrStr,
		Classification: classification,
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
