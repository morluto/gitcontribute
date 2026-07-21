//go:build !windows

package corpus

import "os"

func replaceDatabaseFile(source, destination string) error {
	return os.Rename(source, destination)
}
