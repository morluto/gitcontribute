package similarity

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

const defaultMaxBodyTokens = 1000

// ThreadRef identifies one issue or pull request for exact similarity scoring.
type ThreadRef struct {
	Repo   domain.RepoRef
	Kind   domain.ThreadKind
	Number int
}

// Less reports the canonical ordering of thread references.
func (r ThreadRef) Less(other ThreadRef) bool {
	leftOwner, rightOwner := strings.ToLower(r.Repo.Owner), strings.ToLower(other.Repo.Owner)
	if leftOwner != rightOwner {
		return leftOwner < rightOwner
	}
	leftRepo, rightRepo := strings.ToLower(r.Repo.Repo), strings.ToLower(other.Repo.Repo)
	if leftRepo != rightRepo {
		return leftRepo < rightRepo
	}
	if r.Kind != other.Kind {
		return r.Kind < other.Kind
	}
	return r.Number < other.Number
}

// ThreadText contains the exact local fields consumed by a similarity policy.
type ThreadText struct {
	Ref    ThreadRef
	Title  string
	Body   string
	Labels []string
	Author string
}

// DuplicateRule is the validated duplicate-v1 preparation and scoring policy.
type DuplicateRule struct {
	maxBodyTokens int
}

// DefaultDuplicateRule returns the supported duplicate-v1 scoring policy.
func DefaultDuplicateRule() DuplicateRule {
	return DuplicateRule{maxBodyTokens: defaultMaxBodyTokens}
}

// Version identifies the exact duplicate scoring semantics.
func (DuplicateRule) Version() RuleVersion { return DuplicateV1 }

// Valid reports whether the rule was created by a supported constructor.
func (r DuplicateRule) Valid() bool { return r.maxBodyTokens > 0 }

// PreparedDuplicate is an immutable duplicate-v1 representation of a thread.
type PreparedDuplicate struct {
	ref        ThreadRef
	title      []string
	body       []string
	labels     []string
	author     string
	references []ThreadRef
}

// Prepare normalizes every field once for repeated exact comparisons.
func (r DuplicateRule) Prepare(thread ThreadText) PreparedDuplicate {
	labels := normalizedLabels(thread.Labels)
	return PreparedDuplicate{
		ref:        thread.Ref,
		title:      Tokens(thread.Title, true),
		body:       TokensLimited(thread.Body, true, r.maxBodyTokens),
		labels:     labels,
		author:     thread.Author,
		references: ExtractRefs(thread.Title+"\n"+thread.Body, thread.Ref.Repo),
	}
}

// DuplicateSignals are the explainable components of duplicate-v1 scoring.
type DuplicateSignals struct {
	ExplicitRef  bool
	TitleJaccard float64
	BodyJaccard  float64
	LabelJaccard float64
	SameAuthor   bool
}

// Score returns the weighted duplicate-v1 score.
func (s DuplicateSignals) Score() float64 {
	raw := s.TitleJaccard*0.45 + s.BodyJaccard*0.05 + s.LabelJaccard*0.05
	if s.ExplicitRef {
		raw += 0.40
	}
	if s.SameAuthor {
		raw += 0.05
	}
	return math.Min(1, raw)
}

// Reason explains the strongest duplicate-v1 signals.
func (s DuplicateSignals) Reason() string {
	var parts []string
	if s.ExplicitRef {
		parts = append(parts, "explicit reference")
	}
	if s.TitleJaccard > 0.3 {
		parts = append(parts, fmt.Sprintf("title similarity %.2f", s.TitleJaccard))
	}
	if s.BodyJaccard > 0.1 {
		parts = append(parts, fmt.Sprintf("body similarity %.2f", s.BodyJaccard))
	}
	if s.LabelJaccard > 0 {
		parts = append(parts, fmt.Sprintf("shared labels %.2f", s.LabelJaccard))
	}
	if s.SameAuthor {
		parts = append(parts, "same author")
	}
	if len(parts) == 0 {
		return "no strong signal"
	}
	return strings.Join(parts, "; ")
}

// DuplicateScore is an exact duplicate-v1 score with its explanation and version.
type DuplicateScore struct {
	Value       float64
	Reason      string
	Signals     DuplicateSignals
	RuleVersion RuleVersion
}

// Compare scores two prepared threads exactly under duplicate-v1.
func (r DuplicateRule) Compare(a, b PreparedDuplicate) DuplicateScore {
	signals := DuplicateSignals{
		ExplicitRef:  references(a.references, b.ref) || references(b.references, a.ref),
		TitleJaccard: jaccard(a.title, b.title),
		BodyJaccard:  jaccard(a.body, b.body),
		LabelJaccard: jaccard(a.labels, b.labels),
		SameAuthor:   a.author != "" && a.author == b.author,
	}
	return DuplicateScore{Value: signals.Score(), Reason: signals.Reason(), Signals: signals, RuleVersion: r.Version()}
}

func references(refs []ThreadRef, candidate ThreadRef) bool {
	for _, ref := range refs {
		if ref.Number == candidate.Number && strings.EqualFold(ref.Repo.Owner, candidate.Repo.Owner) && strings.EqualFold(ref.Repo.Repo, candidate.Repo.Repo) && (ref.Kind == "" || ref.Kind == candidate.Kind) {
			return true
		}
	}
	return false
}

func normalizedLabels(labels []string) []string {
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		if normalized := strings.ToLower(strings.TrimSpace(label)); normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for label := range seen {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			intersection++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
