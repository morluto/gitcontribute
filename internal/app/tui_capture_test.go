package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/morluto/gitcontribute/internal/config"
)

func TestCaptureTUIHeroes(t *testing.T) {
	if os.Getenv("UPDATE_TUI_HERO") != "1" {
		t.Skip("set UPDATE_TUI_HERO=1 to recapture real-executable TUI assets")
	}
	for _, tool := range []string{"tmux", "agg", "magick"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("capture tool %s is unavailable: %v", tool, err)
		}
	}

	fixture := newResearchFixture(t)
	home := fixture.svc.paths.HomeDir()
	if err := fixture.svc.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(root, "docs", "assets")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "gitcontribute")
	build := exec.Command("go", "build", "-o", binary, "./cmd/gitcontribute")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build capture executable: %v\n%s", err, output)
	}

	socket := fmt.Sprintf("gitcontribute-capture-%d", time.Now().UnixNano())
	session := "hero"
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
	startTUISession(t, socket, session, home, binary)

	waitForPane(t, socket, session, "GitContribute", "Retry parser cancellation")
	workbench := capturePane(t, socket, session, true)
	renderHero(t, workbench, 118, 36, filepath.Join(outputDir, "gitcontribute-tui-workbench.png"))

	sendKeys(t, socket, session, "Enter")
	waitForPane(t, socket, session, "RESEARCH BRIEF", "Expected behavior")
	brief := capturePane(t, socket, session, true)
	renderHero(t, brief, 118, 36, filepath.Join(outputDir, "gitcontribute-tui-research-brief.png"))

	sendKeys(t, socket, session, "Escape")
	waitForPane(t, socket, session, "Retry parser cancellation", "[a] Start investigation")
	sendKeys(t, socket, session, "a")
	waitForPane(t, socket, session, "Actions", "Start investigation")
	palette := capturePane(t, socket, session, false)
	sendKeys(t, socket, session, "Enter")
	waitForPane(t, socket, session, "Confirm local write", "No network access or GitHub mutation")
	confirmation := capturePane(t, socket, session, false)
	sendKeys(t, socket, session, "y")
	waitForPane(t, socket, session, "ACTION RESULT", "Investigation started", "Seed hypothesis")
	writeProbeEvidence(t, root, "probe-2-action-workflow.txt", strings.Join([]string{
		"=== ACTION PALETTE ===", palette,
		"=== CONFIRMATION ===", confirmation,
		"=== RESULT ===", capturePane(t, socket, session, false),
	}, "\n"))
	sendKeys(t, socket, session, "Enter")
	waitForPane(t, socket, session, "RESEARCH", "parser cancellation")
	sendKeys(t, socket, session, "a")
	waitForPane(t, socket, session, "Find similar threads", "Find competing pull requests")
	sendKeys(t, socket, session, "Enter")
	waitForPane(t, socket, session, "ACTION RESULT", "Duplicate check complete", "Source revision")
	sendKeys(t, socket, session, "Escape")
	waitForPane(t, socket, session, "RESEARCH", "parser cancellation")
	sendKeys(t, socket, session, "q")
	waitForSessionExit(t, socket, session)

	writeProbeEvidence(t, root, "probe-1-populated-workbench.txt", workbench)

	emptyHome := t.TempDir()
	emptyService, err := New(config.NewPaths(&config.Env{Home: emptyHome}), "capture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emptyService.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := emptyService.Close(); err != nil {
		t.Fatal(err)
	}
	emptySession := "empty"
	startTUISession(t, socket, emptySession, emptyHome, binary)
	waitForPane(
		t, socket, emptySession,
		"No contribution candidates yet",
		"The local corpus contains no repositories",
		"gitcontribute source add repos",
		"gitcontribute archive sync",
	)
	writeProbeEvidence(t, root, "probe-3-empty-corpus.txt", capturePane(t, socket, emptySession, false))
	sendKeys(t, socket, emptySession, "q")
	waitForSessionExit(t, socket, emptySession)
}

func startTUISession(t *testing.T, socket, session, home, binary string) {
	t.Helper()
	command := "env HOME=" + shellQuote(home) + " TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 " + shellQuote(binary) + " tui"
	start := exec.Command("tmux", "-L", socket, "-f", "/dev/null", "new-session", "-d", "-x", "118", "-y", "36", "-s", session, command)
	if output, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start capture session: %v\n%s", err, output)
	}
}

func sendKeys(t *testing.T, socket, session string, keys ...string) {
	t.Helper()
	args := append([]string{"-L", socket, "send-keys", "-t", session}, keys...)
	if output, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("send keys %q: %v\n%s", keys, err, output)
	}
}

func waitForSessionExit(t *testing.T, socket, session string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := exec.Command("tmux", "-L", socket, "has-session", "-t", session).Run(); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("terminal session %q did not exit cleanly", session)
}

func writeProbeEvidence(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, ".codex", "tui-contribution-workflow", "evidence", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(ansi.Strip(content))+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForPane(t *testing.T, socket, session string, wants ...string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = capturePane(t, socket, session, false)
		found := true
		for _, want := range wants {
			found = found && strings.Contains(last, want)
		}
		if found {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for terminal content %q:\n%s", wants, last)
}

func capturePane(t *testing.T, socket, session string, escapeCodes bool) string {
	t.Helper()
	args := []string{"-L", socket, "capture-pane", "-p"}
	if escapeCodes {
		args = append(args, "-e")
	}
	args = append(args, "-t", session)
	output, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("capture terminal pane: %v\n%s", err, output)
	}
	return string(output)
}

func renderHero(t *testing.T, frame string, width, height int, outputPath string) {
	t.Helper()
	// A captured terminal row may occupy every source column. Give the playback
	// terminal one spare column so its automatic wrap does not double-space
	// full-width border rows.
	playbackWidth := width + 1
	work := t.TempDir()
	castPath := filepath.Join(work, "frame.cast")
	gifPath := filepath.Join(work, "frame.gif")
	header, err := json.Marshal(map[string]any{
		"version": 2, "width": playbackWidth, "height": height, "timestamp": 0,
		"env": map[string]string{"SHELL": "/bin/sh", "TERM": "xterm-256color"},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame = strings.ReplaceAll(strings.TrimRight(frame, "\n"), "\n", "\r\n")
	event, err := json.Marshal([]any{0.0, "o", "\x1b[2J\x1b[H" + frame})
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Join([][]byte{header, event, nil}, []byte("\n"))
	if err := os.WriteFile(castPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	aggregate := exec.Command(
		"agg", "--cols", fmt.Sprint(playbackWidth), "--rows", fmt.Sprint(height),
		"--font-size", "18", "--line-height", "1.25", "--theme", "nord",
		"--no-loop", castPath, gifPath,
	)
	if output, err := aggregate.CombinedOutput(); err != nil {
		t.Fatalf("render terminal frame: %v\n%s", err, output)
	}
	convert := exec.Command("magick", gifPath+"[0]", "-strip", outputPath)
	if output, err := convert.CombinedOutput(); err != nil {
		t.Fatalf("convert terminal frame to PNG: %v\n%s", err, output)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
