//go:build !windows

package setup

import "os"

func replaceFile(source, destination string) error { return os.Rename(source, destination) }
