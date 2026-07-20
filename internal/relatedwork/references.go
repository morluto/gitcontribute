// Package relatedwork owns dependency-neutral relationship facts extracted
// from stored thread text. It performs no I/O and treats source text as data.
package relatedwork

import (
	"regexp"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

const (
	// RelationExplicitReference is a source-text reference with no stronger
	// closing, dependency, or blocking phrase.
	RelationExplicitReference = "explicit_reference"
	// RelationMentions is an inbound pull-request reference without closing semantics.
	RelationMentions = "mentions"
	// RelationClaimsToClose is an explicit closing-keyword or GitHub closing-issue relationship.
	RelationClaimsToClose = "claims_to_close"
	// RelationDependsOn means the source states that it requires the referenced thread.
	RelationDependsOn = "depends_on"
	// RelationBlocks means the source states that it blocks the referenced thread.
	RelationBlocks = "blocks"
	// RelationCrossReference is an explicit GitHub timeline cross-reference.
	RelationCrossReference = "cross_reference"
	// RelationClusterCandidate is a stored duplicate-cluster relationship.
	RelationClusterCandidate = "cluster_candidate"
)

const githubThreadReference = `((?:(?:https?://)?(?:www\.)?github\.com/[a-z0-9](?:[a-z0-9-]*[a-z0-9])?/[a-z0-9_.-]+/(?:issues|pull)/\d+)|(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?/[a-z0-9_.-]+#\s*\d+)|(?:#\s*\d+))`

var (
	closingReferencePattern = regexp.MustCompile(`(?i)\b(?:close(?:s|d)?|fix(?:es|ed)?|resolve(?:s|d)?)\s*:?\s*` + githubThreadReference)
	dependencyPattern       = regexp.MustCompile(`(?i)\b(?:depends?\s+on|blocked\s+by|requires?|prerequisite\s+is|based\s+on|stacked\s+(?:on|upon)(?:\s+top\s+of)?|built\s+on|after)\s*:?\s*` + githubThreadReference)
	blocksPattern           = regexp.MustCompile(`(?i)\b(?:blocks?|unblocks?|prerequisite\s+for)\s*:?\s*` + githubThreadReference)
)

// Reference is one exact GitHub thread reference and the strongest lexical
// relationship stated for it in the same source text. Kind can be empty for
// GitHub's ambiguous owner/repo#N and #N syntax.
type Reference struct {
	Repo     domain.RepoRef
	Kind     domain.ThreadKind
	Number   int
	Relation string
}

// Extract returns sorted, unique references from unquoted prose. Fenced code,
// inline code, indented code, and blockquotes are excluded so copied examples
// cannot become collision or blocker evidence.
func Extract(text string, defaultRepo domain.RepoRef) []Reference {
	text = unquotedMarkdown(text)
	refs := similarity.ExtractRefs(text, defaultRepo)
	relations := make(map[similarity.ThreadRef]string, len(refs))
	for _, ref := range refs {
		relations[ref] = RelationExplicitReference
	}
	applyRelation := func(pattern *regexp.Regexp, relation string) {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			for _, ref := range similarity.ExtractRefs(match[1], defaultRepo) {
				if Priority(relation) > Priority(relations[ref]) {
					relations[ref] = relation
				}
			}
		}
	}
	applyRelation(blocksPattern, RelationBlocks)
	applyRelation(dependencyPattern, RelationDependsOn)
	applyRelation(closingReferencePattern, RelationClaimsToClose)

	out := make([]Reference, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Reference{Repo: ref.Repo, Kind: ref.Kind, Number: ref.Number, Relation: relations[ref]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo.String() != out[j].Repo.String() {
			return out[i].Repo.String() < out[j].Repo.String()
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Number < out[j].Number
	})
	return out
}

// Priority orders relationship specificity for deterministic de-duplication.
func Priority(relation string) int {
	switch relation {
	case RelationClaimsToClose:
		return 6
	case RelationDependsOn:
		return 5
	case RelationBlocks:
		return 4
	case RelationCrossReference, RelationClusterCandidate:
		return 3
	case RelationMentions:
		return 2
	case RelationExplicitReference:
		return 1
	default:
		return 0
	}
}

func unquotedMarkdown(value string) string {
	lines := strings.Split(value, "\n")
	kept := make([]string, 0, len(lines))
	fenceMarker, fenceLength := byte(0), 0
	for _, line := range lines {
		marker, length, rest, isFence := markdownFence(line)
		if fenceMarker != 0 {
			if marker == fenceMarker && length >= fenceLength && strings.TrimSpace(rest) == "" {
				fenceMarker, fenceLength = 0, 0
			}
			continue
		}
		if isFence {
			fenceMarker, fenceLength = marker, length
			continue
		}
		if markdownBlockquote(line) || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		kept = append(kept, line)
	}
	return stripMarkdownCodeSpans(strings.Join(kept, "\n"))
}

func markdownFence(line string) (marker byte, length int, rest string, ok bool) {
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent >= len(line) || line[indent] != '`' && line[indent] != '~' {
		return 0, 0, "", false
	}
	marker = line[indent]
	end := indent
	for end < len(line) && line[end] == marker {
		end++
	}
	if end-indent < 3 {
		return 0, 0, "", false
	}
	return marker, end - indent, line[end:], true
}

func markdownBlockquote(line string) bool {
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	return indent <= 3 && indent < len(line) && line[indent] == '>'
}

type backtickRun struct {
	start  int
	length int
}

func stripMarkdownCodeSpans(value string) string {
	runs := markdownBacktickRuns(value)
	if len(runs) < 2 {
		return value
	}
	next := make([]int, len(runs))
	last := map[int]int{}
	for i := len(runs) - 1; i >= 0; i-- {
		next[i] = -1
		if following, ok := last[runs[i].length]; ok {
			next[i] = following
		}
		last[runs[i].length] = i
	}
	masked := []byte(value)
	for i := 0; i < len(runs); {
		closing := next[i]
		if closing < 0 {
			i++
			continue
		}
		end := runs[closing].start + runs[closing].length
		for offset := runs[i].start; offset < end; offset++ {
			if masked[offset] != '\n' && masked[offset] != '\r' {
				masked[offset] = ' '
			}
		}
		i = closing + 1
	}
	return string(masked)
}

func markdownBacktickRuns(value string) []backtickRun {
	runs := []backtickRun{}
	for i := 0; i < len(value); {
		if value[i] != '`' {
			i++
			continue
		}
		end := i + 1
		for end < len(value) && value[end] == '`' {
			end++
		}
		runs = append(runs, backtickRun{start: i, length: end - i})
		i = end
	}
	return runs
}
