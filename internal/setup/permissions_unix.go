//go:build !windows

package setup

import "os"

func registrationModeMatchesActivation(mode os.FileMode) bool {
	return mode.Perm() == 0o600
}

func restoreRegistrationMode(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}
