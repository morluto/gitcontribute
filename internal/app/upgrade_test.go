package app

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

func TestUpgradeNpxDoesNotInstallGlobalPackage(t *testing.T) {
	original := upgradeCommand
	t.Cleanup(func() { upgradeCommand = original })
	var calls [][]string
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte("1.2.4\n"), nil
	}
	t.Setenv("npm_command", "exec")
	svc := &Service{version: "1.2.3"}
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Context != "npx" || len(calls) != 1 {
		t.Fatalf("report=%+v calls=%v", report, calls)
	}
	want := []string{"npm", "view", "gitcontribute", "version"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("call = %v, want %v", calls[0], want)
	}
}

func TestDetectInstallContextDefaultsOther(t *testing.T) {
	t.Setenv("npm_command", "")
	t.Setenv("npm_lifecycle_event", "")
	if got := detectInstallContext(context.Background()); got == "npx" {
		t.Fatalf("context = %s", got)
	}
}

func TestClassifyNPMExecutableDistinguishesProjectAndGlobalInstalls(t *testing.T) {
	globalRoot := filepath.Join(string(filepath.Separator), "usr", "local", "lib", "node_modules")
	global := filepath.Join(globalRoot, "gitcontribute", "npm", "bin", "native", "linux-x64", "gitcontribute")
	project := filepath.Join(string(filepath.Separator), "work", "project", "node_modules", "gitcontribute", "npm", "bin", "native", "linux-x64", "gitcontribute")
	if got := classifyNPMExecutable(global, globalRoot); got != "global-npm" {
		t.Fatalf("global context = %q", got)
	}
	if got := classifyNPMExecutable(project, globalRoot); got != "project-npm" {
		t.Fatalf("project context = %q", got)
	}
}
