package corpus

// SupportedSchemaVersion returns the newest embedded corpus schema without
// opening a database or inspecting the filesystem.
func SupportedSchemaVersion() (int64, error) {
	return latestSchemaVersion()
}
