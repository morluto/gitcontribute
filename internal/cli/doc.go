// Package cli parses command-line input and renders stable human and JSON
// output for GitContribute application services.
//
// The package defines adapter-facing request and result contracts but does not
// own persistence, network, or contribution-workflow decisions. Commands make
// side effects visible through their names and flags.
package cli
