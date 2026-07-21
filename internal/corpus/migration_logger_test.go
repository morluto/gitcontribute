package corpus

// noopLogger keeps migration fixture output quiet. Production migrations use
// migrationLogger, which converts fatal callbacks into returned errors.
type noopLogger struct{}

func (noopLogger) Printf(string, ...interface{}) {}
func (noopLogger) Fatalf(string, ...interface{}) {}
