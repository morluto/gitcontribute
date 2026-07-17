package tracking

import (
	"regexp"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

var (
	keyValuePattern     = regexp.MustCompile(`(?i)["']?[a-z_]*(?:token|secret|password|api[-_]?key|auth[-_]?token)[a-z_]*["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:Bearer|Basic|token)\s+[^\s,;}\]]+|[^\s,;}\]]+)`)
	authHeaderPattern   = regexp.MustCompile(`(?i)(Authorization\s*:\s*(?:Bearer|token|Token|Basic)\s+)(\S+)`)
	legacyGitHubPat     = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`)
	fineGrainedPat      = regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`)
	absPathPattern      = regexp.MustCompile(`(?i)(^|[\s"'=(])(/[A-Za-z0-9_.-][^"'\r\n,;}\]]*|[A-Za-z]:\\[^"'\r\n,;}\]]*)`)
	keyComponentPattern = regexp.MustCompile(`[A-Za-z0-9]+`)
)

var sensitiveKeyComponents = map[string]struct{}{
	"token": {}, "secret": {}, "password": {}, "authorization": {},
	"credential": {}, "signature": {}, "apikey": {}, "authtoken": {},
	"accesstoken": {}, "clientsecret": {}, "clientid": {},
	"privatekey": {}, "githubpat": {},
}

// OrderBundle sorts each slice so the same records produce the same JSON.
func OrderBundle(bundle *Bundle) {
	sort.SliceStable(bundle.TriageEvents, func(i, j int) bool {
		a, b := bundle.TriageEvents[i], bundle.TriageEvents[j]
		if !a.SourceEventAt.Equal(b.SourceEventAt) {
			return a.SourceEventAt.Before(b.SourceEventAt)
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	sort.SliceStable(bundle.Contributions, func(i, j int) bool {
		a, b := bundle.Contributions[i], bundle.Contributions[j]
		if !a.PreparedAt.Equal(b.PreparedAt) {
			return a.PreparedAt.Before(b.PreparedAt)
		}
		return a.ID < b.ID
	})
	sort.SliceStable(bundle.ContributionOutcomes, func(i, j int) bool {
		a, b := bundle.ContributionOutcomes[i], bundle.ContributionOutcomes[j]
		if !a.SourceEventAt.Equal(b.SourceEventAt) {
			return a.SourceEventAt.Before(b.SourceEventAt)
		}
		return a.ID < b.ID
	})
	sort.SliceStable(bundle.Evidence, func(i, j int) bool {
		a, b := bundle.Evidence[i], bundle.Evidence[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
}

// SanitizeBundle returns a deep copy of bundle with secrets, credentials, and
// absolute local paths redacted.
func SanitizeBundle(bundle *Bundle) *Bundle {
	if bundle == nil {
		return nil
	}
	out := &Bundle{
		SchemaVersion:        bundle.SchemaVersion,
		TriageEvents:         make([]*TriageEvent, len(bundle.TriageEvents)),
		Contributions:        make([]*Contribution, len(bundle.Contributions)),
		ContributionOutcomes: make([]*ContributionOutcome, len(bundle.ContributionOutcomes)),
		Evidence:             make([]*evidence.Evidence, len(bundle.Evidence)),
	}
	for i, e := range bundle.TriageEvents {
		copy := *e
		copy.Reason = sanitizeString(copy.Reason)
		copy.Lens = sanitizeString(copy.Lens)
		copy.TargetRef = sanitizeString(copy.TargetRef)
		out.TriageEvents[i] = &copy
	}
	for i, c := range bundle.Contributions {
		copy := *c
		copy.Title = sanitizeString(copy.Title)
		copy.Body = sanitizeString(copy.Body)
		copy.Reference = sanitizeString(copy.Reference)
		copy.ReferenceURL = sanitizeString(copy.ReferenceURL)
		copy.Metadata = sanitizeMetadata(copy.Metadata)
		out.Contributions[i] = &copy
	}
	for i, o := range bundle.ContributionOutcomes {
		copy := *o
		copy.Reason = sanitizeString(copy.Reason)
		out.ContributionOutcomes[i] = &copy
	}
	for i, item := range bundle.Evidence {
		copy := *item
		copy.Description = sanitizeString(copy.Description)
		copy.SourceRefs = append([]domain.SourceRef(nil), copy.SourceRefs...)
		for j := range copy.SourceRefs {
			copy.SourceRefs[j].Source = sanitizeString(copy.SourceRefs[j].Source)
			copy.SourceRefs[j].URL = sanitizeString(copy.SourceRefs[j].URL)
			copy.SourceRefs[j].CommitSHA = sanitizeString(copy.SourceRefs[j].CommitSHA)
		}
		copy.SourceProvenance = append([]evidence.SourceRevision(nil), copy.SourceProvenance...)
		out.Evidence[i] = &copy
	}
	return out
}

func sanitizeString(s string) string {
	if s == "" {
		return ""
	}
	s = keyValuePattern.ReplaceAllStringFunc(s, redactKeyValueMatch)
	s = authHeaderPattern.ReplaceAllString(s, "${1}[REDACTED]")
	s = fineGrainedPat.ReplaceAllString(s, "[REDACTED]")
	s = legacyGitHubPat.ReplaceAllString(s, "[REDACTED]")
	s = absPathPattern.ReplaceAllStringFunc(s, redactPathMatch)
	return s
}

func redactKeyValueMatch(m string) string {
	for i, r := range m {
		if r == ':' || r == '=' {
			return strings.TrimRight(m[:i+1], " \t") + " [REDACTED]"
		}
	}
	return "[REDACTED]"
}

func redactPathMatch(m string) string {
	parts := absPathPattern.FindStringSubmatch(m)
	if len(parts) != 3 {
		return m
	}
	return parts[1] + "[REDACTED_PATH]"
}

func sanitizeMetadata(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = sanitizeValue(v)
	}
	return out
}

func isSensitiveKey(k string) bool {
	components := keyComponentPattern.FindAllString(k, -1)
	for _, component := range components {
		if _, ok := sensitiveKeyComponents[strings.ToLower(component)]; ok {
			return true
		}
	}
	compact := strings.ToLower(strings.Join(components, ""))
	_, ok := sensitiveKeyComponents[compact]
	return ok
}

func sanitizeValue(v any) any {
	switch x := v.(type) {
	case string:
		return sanitizeString(x)
	case map[string]any:
		return sanitizeMetadata(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = sanitizeValue(item)
		}
		return out
	default:
		return x
	}
}
