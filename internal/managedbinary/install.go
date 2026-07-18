// Package managedbinary owns GitContribute's private MCP runtime installation.
package managedbinary

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var runtimeVersion = regexp.MustCompile(`^(dev|[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

// Destination returns the versioned private executable path under dataDir.
func Destination(dataDir, version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !runtimeVersion.MatchString(version) {
		return "", fmt.Errorf("invalid managed runtime version %q", version)
	}
	name := "gitcontribute"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dataDir, "bin", version, name), nil
}

// Install atomically copies source to destination as an executable file. It
// returns false when an identical managed executable was already present.
func Install(source, destination string) (bool, error) {
	matching, err := sameContents(source, destination)
	if err != nil {
		return false, err
	}
	if matching {
		if err := os.Chmod(destination, 0o755); err != nil {
			return false, fmt.Errorf("make managed binary executable: %w", err)
		}
		return false, nil
	}
	in, err := os.Open(source)
	if err != nil {
		return false, fmt.Errorf("open packaged executable: %w", err)
	}
	defer in.Close()

	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("create managed binary directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".gitcontribute-runtime-*")
	if err != nil {
		return false, fmt.Errorf("create managed binary: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		return false, fmt.Errorf("copy managed binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		return false, fmt.Errorf("make managed binary executable: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return false, fmt.Errorf("sync managed binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close managed binary: %w", err)
	}
	if err := replaceFile(tmpPath, destination); err != nil {
		return false, fmt.Errorf("activate managed binary: %w", err)
	}
	cleanup = false
	return true, nil
}

func sameContents(source, destination string) (bool, error) {
	destinationInfo, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect managed binary: %w", err)
	}
	if !destinationInfo.Mode().IsRegular() {
		return false, nil
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false, fmt.Errorf("inspect packaged executable: %w", err)
	}
	if sourceInfo.Size() != destinationInfo.Size() {
		return false, nil
	}
	sourceHash, err := fileHash(source)
	if err != nil {
		return false, err
	}
	destinationHash, err := fileHash(destination)
	if err != nil {
		return false, err
	}
	return sourceHash == destinationHash, nil
}

func fileHash(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("open executable for comparison: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("hash executable: %w", err)
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}
