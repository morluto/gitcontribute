//go:build windows

package managedbinary

import "os"

func replaceFile(source, destination string) error {
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, destination)
}
