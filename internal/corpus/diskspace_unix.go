//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package corpus

import (
	"fmt"
	"math"
	"strconv"

	"golang.org/x/sys/unix"
)

func freeDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	blocks, err := parseNonNegativeDiskValue(stat.Bavail)
	if err != nil {
		return 0, err
	}
	blockSize, err := parseNonNegativeDiskValue(stat.Bsize)
	if err != nil {
		return 0, err
	}
	if blockSize != 0 && blocks > math.MaxUint64/blockSize {
		return 0, strconv.ErrRange
	}
	return blocks * blockSize, nil
}

func parseNonNegativeDiskValue[T ~int64 | ~uint64](value T) (uint64, error) {
	return strconv.ParseUint(fmt.Sprint(value), 10, 64)
}
