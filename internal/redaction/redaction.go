// Package redaction removes common credential forms from content crossing a
// publication boundary. It is deliberately small and deterministic.
package redaction

import (
	"regexp"
	"strings"
)

var (
	keyValuePattern   = regexp.MustCompile(`(?i)["']?[a-z_]*(?:token|secret|password|api[-_]?key|auth[-_]?token)[a-z_]*["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:Bearer|Basic|token)\s+[^\s,;}\]]+|[^\s,;}\]]+)`)
	authHeaderPattern = regexp.MustCompile(`(?i)(Authorization\s*:\s*(?:Bearer|token|Token|Basic)\s+)(\S+)`)
	legacyGitHubPat   = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`)
	fineGrainedPat    = regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`)
)

// String replaces common credentials with a stable marker.
func String(s string) string {
	if s == "" {
		return ""
	}
	s = keyValuePattern.ReplaceAllStringFunc(s, redactKeyValueMatch)
	s = authHeaderPattern.ReplaceAllString(s, "${1}[REDACTED]")
	s = fineGrainedPat.ReplaceAllString(s, "[REDACTED]")
	s = legacyGitHubPat.ReplaceAllString(s, "[REDACTED]")
	return s
}

func redactKeyValueMatch(match string) string {
	for i, r := range match {
		if r == ':' || r == '=' {
			return strings.TrimRight(match[:i+1], " \t") + " [REDACTED]"
		}
	}
	return "[REDACTED]"
}
