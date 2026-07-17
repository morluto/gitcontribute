// Package workspace manages Git mirrors and detached worktrees used to inspect
// candidate contributions.
//
// Paths are contained beneath one managed root, repository hooks and optional
// helpers are disabled, command output is bounded, and dirty worktrees require
// explicit force before removal. Creating a workspace invokes git but never
// executes repository-controlled code.
package workspace
