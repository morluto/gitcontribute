package managedbinary

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestListReturnsUsableVersionedRuntimesInSemanticOrder(t *testing.T) {
	dataDir := t.TempDir()
	for _, version := range []string{"1.10.0", "1.2.0", "1.2.0-rc.1", "ignored"} {
		path, err := Destination(dataDir, version)
		if err != nil {
			if version == "ignored" {
				if err := os.MkdirAll(filepath.Join(dataDir, "bin", version), 0o700); err != nil {
					t.Fatal(err)
				}
				continue
			}
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(version), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	runtimes, err := List(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{runtimes[0].Version, runtimes[1].Version, runtimes[2].Version}; !reflect.DeepEqual(got, []string{"1.2.0-rc.1", "1.2.0", "1.10.0"}) {
		t.Fatalf("versions = %v", got)
	}
}

func TestDestinationUsesSafeVersionedPath(t *testing.T) {
	destination, err := Destination(filepath.Join("data", "gitcontribute"), "v1.2.3-beta.1+build.7")
	if err != nil {
		t.Fatal(err)
	}
	name := "gitcontribute"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	want := filepath.Join("data", "gitcontribute", "bin", "1.2.3-beta.1+build.7", name)
	if destination != want {
		t.Fatalf("destination = %q, want %q", destination, want)
	}
}

func TestDestinationRejectsUnsafeVersion(t *testing.T) {
	for _, version := range []string{"", "latest", "../1.2.3", "1.2", "1.2.3/other"} {
		t.Run(strings.ReplaceAll(version, "/", "_"), func(t *testing.T) {
			if _, err := Destination(t.TempDir(), version); err == nil {
				t.Fatalf("Destination accepted %q", version)
			}
		})
	}
}

func TestInstallCopiesExecutableAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "packaged")
	destination := filepath.Join(dir, "managed", "gitcontribute")
	if err := os.WriteFile(source, []byte("native-binary"), 0o600); err != nil {
		t.Fatal(err)
	}

	installed, err := Install(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("first install reported an existing binary")
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "native-binary" {
		t.Fatalf("contents = %q", contents)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("permissions = %o, want 755", info.Mode().Perm())
		}
	}

	installed, err = Install(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("identical repeat install rewrote the managed binary")
	}
}

func TestInstallReplacesDestinationSymlinkWithoutTouchingItsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "packaged")
	target := filepath.Join(dir, "unrelated")
	destination := filepath.Join(dir, "managed", "gitcontribute")
	if err := os.WriteFile(source, []byte("same-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("same-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	installed, err := Install(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("destination symlink was treated as an installed binary")
	}
	destinationInfo, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if destinationInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("destination remains a symlink")
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if targetInfo.Mode().Perm() != 0o600 {
		t.Fatalf("symlink target permissions = %o, want 600", targetInfo.Mode().Perm())
	}
}
