package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type revisionReaderFunc func(context.Context, SourceSubject) (*SourceRevision, error)

func (f revisionReaderFunc) CurrentSourceRevision(ctx context.Context, subject SourceSubject) (*SourceRevision, error) {
	return f(ctx, subject)
}

func TestFreshnessEvaluatorStatuses(t *testing.T) {
	t.Parallel()
	recorded := testSourceRevision(SourceSubjectThread, "issue", 42, "", time.Unix(100, 0).UTC(), 7)
	tests := []struct {
		name    string
		item    *Evidence
		current *SourceRevision
		want    FreshnessStatus
		reason  string
	}{
		{name: "fresh", item: &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{recorded}}, current: sourceRevisionPtr(recorded), want: FreshnessFresh, reason: "match current"},
		{name: "newer timestamp", item: &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{recorded}}, current: sourceRevisionPtr(testSourceRevision(SourceSubjectThread, "issue", 42, "", time.Unix(101, 0).UTC(), 8)), want: FreshnessStale, reason: "advanced from"},
		{name: "equal timestamp newer sequence", item: &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{recorded}}, current: sourceRevisionPtr(testSourceRevision(SourceSubjectThread, "issue", 42, "", recorded.SourceUpdatedAt, 8)), want: FreshnessStale, reason: "sequence=8"},
		{name: "missing current", item: &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{recorded}}, want: FreshnessUnknown, reason: "unavailable"},
		{name: "current predates export", item: &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{recorded}}, current: sourceRevisionPtr(testSourceRevision(SourceSubjectThread, "issue", 42, "", time.Unix(99, 0).UTC(), 100)), want: FreshnessUnknown, reason: "predates"},
		{name: "local evidence", item: &Evidence{Type: EvidenceTypeManualObservation}, want: FreshnessNotApplicable, reason: "local evidence"},
		{name: "missing source provenance", item: &Evidence{Type: EvidenceTypeGitHubSource}, want: FreshnessUnknown, reason: "no recorded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := NewFreshnessEvaluator(revisionReaderFunc(func(context.Context, SourceSubject) (*SourceRevision, error) {
				return tt.current, nil
			}))
			got, err := evaluator.Evaluate(context.Background(), tt.item)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got.Status != tt.want || !strings.Contains(got.Reason, tt.reason) {
				t.Fatalf("freshness = %+v, want %s containing %q", got, tt.want, tt.reason)
			}
		})
	}
}

func TestFreshnessEvaluatorReasonIsDeterministic(t *testing.T) {
	t.Parallel()
	a := testSourceRevision(SourceSubjectFacet, "issue", 1, "issue_comments", time.Unix(100, 0).UTC(), 4)
	b := testSourceRevision(SourceSubjectFacet, "issue", 1, "pr_reviews", time.Unix(100, 0).UTC(), 5)
	reader := revisionReaderFunc(func(_ context.Context, subject SourceSubject) (*SourceRevision, error) {
		current := a
		if subject.Facet() == b.Subject.Facet() {
			current = b
		}
		current.ObservationSequence += 10
		return &current, nil
	})
	evaluator := NewFreshnessEvaluator(reader)
	first, err := evaluator.Evaluate(context.Background(), &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{b, a}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluator.Evaluate(context.Background(), &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("reason depends on input order:\n%+v\n%+v", first, second)
	}
	if !strings.Contains(first.Reason, "issue_comments") || !strings.Contains(first.Reason, "pr_reviews") {
		t.Fatalf("reason omitted stale sources: %q", first.Reason)
	}
}

func TestNormalizeSourceRevisionsRejectsInvalidAndDuplicateSubjects(t *testing.T) {
	t.Parallel()
	revision := testSourceRevision(SourceSubjectThread, "issue", 1, "", time.Unix(100, 0).UTC(), 1)
	if _, err := NormalizeSourceRevisions([]SourceRevision{revision, revision}); err == nil {
		t.Fatal("duplicate subject accepted")
	}
	revision.ObservedAt = time.Time{}
	if _, err := NormalizeSourceRevisions([]SourceRevision{revision}); err == nil {
		t.Fatal("missing observed_at accepted")
	}
}

func TestParseSourceSubjectRejectsMixedVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		kind, threadKind, facet string
		number                  int
	}{
		{name: "repository thread", kind: "repository", threadKind: "issue", number: 1},
		{name: "thread without number", kind: "thread", threadKind: "issue"},
		{name: "facet without name", kind: "facet"},
		{name: "guidance thread", kind: "guidance", threadKind: "issue", number: 1},
		{name: "unknown kind", kind: "branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseSourceSubject(tt.kind, "owner", "repo", tt.threadKind, tt.number, tt.facet); err == nil {
				t.Fatal("mixed or unsupported source subject accepted")
			}
		})
	}
	if err := (SourceSubject{}).Validate(); err == nil {
		t.Fatal("unparsed source subject accepted")
	}
}

func TestSourceSubjectJSONParsesAndCanonicalizes(t *testing.T) {
	t.Parallel()
	var thread SourceSubject
	if err := json.Unmarshal([]byte(`{"kind":"thread","owner":" Owner ","repo":" Repo ","thread_kind":" issue ","number":42}`), &thread); err != nil {
		t.Fatal(err)
	}
	kind, number, ok := thread.Thread()
	if thread.Kind() != SourceSubjectThread || thread.Repository().String() != "Owner/Repo" || !ok || kind != "issue" || number != 42 {
		t.Fatalf("parsed thread subject = %v", thread)
	}

	var guidance SourceSubject
	if err := json.Unmarshal([]byte(`{"kind":"guidance","owner":"Owner","repo":"Repo","facet":"contribution_guidance"}`), &guidance); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(guidance)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"kind":"guidance","owner":"Owner","repo":"Repo"}`; got != want {
		t.Fatalf("canonical guidance JSON = %s, want %s", got, want)
	}
}

func TestFreshnessEvaluatorHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	evaluator := NewFreshnessEvaluator(revisionReaderFunc(func(context.Context, SourceSubject) (*SourceRevision, error) {
		called = true
		return nil, errors.New("unexpected")
	}))
	_, err := evaluator.Evaluate(ctx, &Evidence{Type: EvidenceTypeGitHubSource, SourceProvenance: []SourceRevision{
		testSourceRevision(SourceSubjectRepository, "", 0, "", time.Unix(100, 0).UTC(), 1),
	}})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("Evaluate = %v, reader called=%v", err, called)
	}
}

func testSourceRevision(kind SourceSubjectKind, threadKind string, number int, facet string, updated time.Time, sequence int64) SourceRevision {
	subject, err := ParseSourceSubject(kind.String(), "Owner", "Repo", threadKind, number, facet)
	if err != nil {
		panic(err)
	}
	return SourceRevision{
		Subject:         subject,
		SourceUpdatedAt: updated, ObservationSequence: sequence, ObservedAt: time.Unix(200, 0).UTC(),
	}
}

func sourceRevisionPtr(revision SourceRevision) *SourceRevision { return &revision }
