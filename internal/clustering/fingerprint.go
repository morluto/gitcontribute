package clustering

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/morluto/gitcontribute/internal/domain"
)

// stopWords filters the most common English words that otherwise match
// almost every candidate. It is intentionally small and deterministic.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "have": {}, "has": {}, "had": {}, "do": {},
	"does": {}, "did": {}, "will": {}, "would": {}, "could": {}, "should": {},
	"may": {}, "might": {}, "must": {}, "shall": {}, "can": {}, "need": {}, "ought": {},
	"to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {}, "at": {}, "by": {},
	"from": {}, "as": {}, "into": {}, "through": {}, "and": {}, "or": {}, "but": {},
	"so": {}, "yet": {}, "if": {}, "because": {}, "although": {}, "though": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "i": {}, "you": {}, "he": {},
	"she": {}, "it": {}, "we": {}, "they": {}, "me": {}, "him": {}, "her": {}, "us": {},
	"them": {}, "my": {}, "your": {}, "his": {}, "its": {}, "our": {}, "their": {},
}

// NormalizeText lowercases and removes characters that are not letters,
// digits, or whitespace, then collapses whitespace.
func NormalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Tokens returns sorted unique tokens from normalized text, filtering stop
// words and single-character tokens.
func Tokens(text string, stop bool) []string {
	return TokensLimited(text, stop, 0)
}

// TokensLimited returns sorted unique tokens with an optional hard limit on the
// number of input words processed. A limit of 0 means unlimited.
func TokensLimited(text string, stop bool, maxWords int) []string {
	norm := NormalizeText(text)
	fields := strings.Fields(norm)
	if maxWords > 0 && len(fields) > maxWords {
		fields = fields[:maxWords]
	}
	seen := make(map[string]struct{}, len(fields))
	for _, w := range fields {
		if len(w) <= 1 {
			continue
		}
		if stop {
			if _, ok := stopWords[w]; ok {
				continue
			}
		}
		seen[w] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

var (
	// Matches owner/repo#123 or owner/repo #123 (issue or pull-request number).
	repoIssueRefPattern = regexp.MustCompile(`(?i)([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)/([a-z0-9_.-]+)\s*#\s*(\d+)`)
	// Matches github.com/owner/repo/issues/123 or pull/123.
	urlRefPattern = regexp.MustCompile(`(?i)github\.com/([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)/([a-z0-9_.-]+)/(?:issues|pull)/(\d+)`)
	// Matches bare #123 in body or title, constrained to word boundaries.
	bareRefPattern = regexp.MustCompile(`(?:^|\s|\W)#\s*(\d+)(?:\b|$)`)
)

// ExtractRefs returns explicit GitHub issue/PR references found in text.
// Bare #123 references inherit the default repository and an empty kind.
func ExtractRefs(text string, defaultRepo domain.RepoRef) []MemberRef {
	seen := make(map[MemberRef]struct{})

	add := func(owner, repo, kind string, number int) {
		if owner == "" || repo == "" || number == 0 {
			return
		}
		ref := MemberRef{Owner: owner, Repo: repo, Kind: kind, Number: number}
		seen[ref] = struct{}{}
	}

	for _, m := range repoIssueRefPattern.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[3])
		add(strings.ToLower(m[1]), strings.ToLower(m[2]), "", n)
	}
	for _, m := range urlRefPattern.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[3])
		kind := "issue"
		if strings.Contains(strings.ToLower(m[0]), "/pull/") {
			kind = "pull_request"
		}
		add(strings.ToLower(m[1]), strings.ToLower(m[2]), kind, n)
	}
	for _, m := range bareRefPattern.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		add(strings.ToLower(defaultRepo.Owner), strings.ToLower(defaultRepo.Repo), "", n)
	}

	out := make([]MemberRef, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sortMemberRefs(out)
	return out
}
