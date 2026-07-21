//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package corpus

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func freeDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	blocks := uint64(stat.Bavail)
	blockSize := uint64(stat.Bsize)
	if blockSize != 0 && blocks > math.MaxUint64/blockSize {
		return 0, fmt.Errorf("available disk space overflows uint64")
	}
	return blocks * blockSize, nil
}
