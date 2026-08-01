// Package managedbinary owns GitContribute's private MCP runtime installation.
package managedbinary

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

var runtimeVersion = regexp.MustCompile(`^(dev|[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

// InstalledRuntime identifies one usable versioned private executable.
type InstalledRuntime struct {
	Version string
	Path    string
}

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

// List returns usable versioned private runtimes under dataDir. Missing data
// directories are treated as an empty installation; malformed or non-regular
// entries are ignored so one abandoned directory cannot hide valid runtimes.
func List(dataDir string) ([]InstalledRuntime, error) {
	entries, err := os.ReadDir(filepath.Join(dataDir, "bin"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list managed runtimes: %w", err)
	}
	runtimes := make([]InstalledRuntime, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !runtimeVersion.MatchString(entry.Name()) {
			continue
		}
		path, err := Destination(dataDir, entry.Name())
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect managed runtime %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
			continue
		}
		runtimes = append(runtimes, InstalledRuntime{Version: entry.Name(), Path: path})
	}
	sort.Slice(runtimes, func(i, j int) bool {
		return compareRuntimeVersions(runtimes[i].Version, runtimes[j].Version) < 0
	})
	return runtimes, nil
}

func compareRuntimeVersions(left, right string) int {
	if left == "dev" {
		if right == "dev" {
			return 0
		}
		return -1
	}
	if right == "dev" {
		return 1
	}
	return semver.Compare("v"+left, "v"+right)
}

// Install atomically copies source to destination as an executable file. It
// returns false when an identical managed executable was already present.
func Install(source, destination string) (bool, error) {
	matching, err := sameContents(source, destination)
	if err != nil {
		return false, err
	}
	if matching {
		// #nosec G302 -- the managed native program must remain executable.
		if err := os.Chmod(destination, 0o755); err != nil {
			return false, fmt.Errorf("make managed binary executable: %w", err)
		}
		return false, nil
	}
	// #nosec G304 -- source is the running executable selected by the application.
	in, err := os.Open(source)
	if err != nil {
		return false, fmt.Errorf("open packaged executable: %w", err)
	}

	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		if closeErr := in.Close(); closeErr != nil {
			err = fmt.Errorf("%w; close packaged executable: %w", err, closeErr)
		}
		return false, fmt.Errorf("create managed binary directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".gitcontribute-runtime-*")
	if err != nil {
		if closeErr := in.Close(); closeErr != nil {
			err = fmt.Errorf("%w; close packaged executable: %w", err, closeErr)
		}
		return false, fmt.Errorf("create managed binary: %w", err)
	}
	tmpPath := tmp.Name()
	_, copyErr := io.Copy(tmp, in)
	closeSourceErr := in.Close()
	if copyErr != nil || closeSourceErr != nil {
		return false, discardTempFile(tmp, tmpPath, fmt.Errorf("copy managed binary: %w", errors.Join(copyErr, closeSourceErr)))
	}
	if err := tmp.Chmod(0o755); err != nil {
		return false, discardTempFile(tmp, tmpPath, fmt.Errorf("make managed binary executable: %w", err))
	}
	if err := tmp.Sync(); err != nil {
		return false, discardTempFile(tmp, tmpPath, fmt.Errorf("sync managed binary: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return false, errors.Join(fmt.Errorf("close managed binary: %w", err), removeTempFile(tmpPath))
	}
	if err := replaceFile(tmpPath, destination); err != nil {
		return false, errors.Join(fmt.Errorf("activate managed binary: %w", err), removeTempFile(tmpPath))
	}
	return true, nil
}

func discardTempFile(file *os.File, path string, cause error) error {
	return errors.Join(cause, file.Close(), removeTempFile(path))
}

func removeTempFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
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
	// #nosec G304 -- callers provide only resolved source and managed destinations.
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("open executable for comparison: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return [sha256.Size]byte{}, fmt.Errorf("hash executable: %w", errors.Join(copyErr, closeErr))
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}
