//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows)

package corpus

import "fmt"

func freeDiskBytes(path string) (uint64, error) {
	return 0, fmt.Errorf("disk space inspection is unsupported for %s", path)
}
