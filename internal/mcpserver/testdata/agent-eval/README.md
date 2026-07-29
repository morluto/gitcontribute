# Agent-tool evaluation fixtures

`v5/` evaluates the unified catalog under eager and host-search conditions. It
records context actually placed before the model separately from deterministic
serialized-byte proxies and keeps its semantic oracle out of band.

Historical fixtures are available from Git history rather than carried as
active test data. Add a new version only for a current product decision with a
reproducible model-in-the-loop protocol; use ordinary Go tests for deterministic
MCP contract coverage.

Do not add credentials, live GitHub responses, wall-clock timings, or claims
about model success to committed fixtures.
