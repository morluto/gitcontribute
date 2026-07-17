// Package app owns GitContribute use cases and capability composition.
//
// Service methods are the shared application boundary for the CLI, MCP server,
// and terminal UI. Methods that read the corpus remain offline; methods that
// access GitHub, write local state, invoke git, or execute validation commands
// expose those effects explicitly. Product rules belong here rather than in a
// presentation adapter.
package app
