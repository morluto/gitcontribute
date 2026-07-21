package setup

import "os"

// Windows does not expose Unix permission bits consistently through FileMode.
// The semantic GitContribute-entry comparison remains the rollback conflict
// boundary; permission restoration is not meaningful on this platform.
func registrationModeMatchesActivation(os.FileMode) bool {
	return true
}

func restoreRegistrationMode(string, os.FileMode) error {
	return nil
}
