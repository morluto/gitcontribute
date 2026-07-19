package similarity

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/morluto/gitcontribute/internal/domain"
)

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

var (
	repoIssueRefPattern = regexp.MustCompile(`(?i)([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)/([a-z0-9_.-]+)\s*#\s*(\d+)`)
	urlRefPattern       = regexp.MustCompile(`(?i)(?:^|[^a-z0-9.-])(?:https?://)?(?:www\.)?github\.com/([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)/([a-z0-9_.-]+)/(?:issues|pull)/(\d+)`)
	bareRefPattern      = regexp.MustCompile(`(?:^|\s|\W)#\s*(\d+)(?:\b|$)`)
)

// NormalizeText lowercases text, replaces punctuation with spaces, and collapses whitespace.
func NormalizeText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

// Tokens returns sorted unique tokens using the duplicate-v1 normalization policy.
func Tokens(text string, filterStopWords bool) []string {
	return TokensLimited(text, filterStopWords, 0)
}

// TokensLimited returns sorted unique tokens after processing at most maxWords input words.
func TokensLimited(text string, filterStopWords bool, maxWords int) []string {
	fields := strings.Fields(NormalizeText(text))
	if maxWords > 0 && len(fields) > maxWords {
		fields = fields[:maxWords]
	}
	seen := make(map[string]struct{}, len(fields))
	for _, word := range fields {
		if len(word) <= 1 {
			continue
		}
		if filterStopWords {
			if _, excluded := stopWords[word]; excluded {
				continue
			}
		}
		seen[word] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for word := range seen {
		out = append(out, word)
	}
	sort.Strings(out)
	return out
}

// ExtractRefs returns sorted, unique GitHub thread references found in text.
func ExtractRefs(text string, defaultRepo domain.RepoRef) []ThreadRef {
	seen := make(map[ThreadRef]struct{})
	add := func(owner, repo string, kind domain.ThreadKind, number int) {
		if owner == "" || repo == "" || number == 0 {
			return
		}
		seen[ThreadRef{Repo: domain.RepoRef{Owner: owner, Repo: repo}, Kind: kind, Number: number}] = struct{}{}
	}
	for _, match := range repoIssueRefPattern.FindAllStringSubmatch(text, -1) {
		number, err := strconv.Atoi(match[3])
		if err != nil {
			continue
		}
		add(strings.ToLower(match[1]), strings.ToLower(match[2]), "", number)
	}
	for _, match := range urlRefPattern.FindAllStringSubmatch(text, -1) {
		number, err := strconv.Atoi(match[3])
		if err != nil {
			continue
		}
		kind := domain.IssueKind
		if strings.Contains(strings.ToLower(match[0]), "/pull/") {
			kind = domain.PullRequestKind
		}
		add(strings.ToLower(match[1]), strings.ToLower(match[2]), kind, number)
	}
	for _, match := range bareRefPattern.FindAllStringSubmatch(text, -1) {
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		add(strings.ToLower(defaultRepo.Owner), strings.ToLower(defaultRepo.Repo), "", number)
	}
	out := make([]ThreadRef, 0, len(seen))
	for ref := range seen {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	return out
}
