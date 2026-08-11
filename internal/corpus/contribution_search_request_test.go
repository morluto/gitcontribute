package corpus

import (
	"strings"
	"testing"
	"time"
)

func TestParseContributionSearchCanonicalizesBoundaryValues(t *testing.T) {
	t.Parallel()
	request, err := ParseContributionSearch(ContributionSearchInput{
		ActorRefs:      []string{" alice ", "alice", "U_1"},
		RepositoryRefs: []string{" Acme/Rocket ", "acme/rocket"},
		Kinds:          []string{" issue ", "issue", "future_kind"},
		Source:         " github_profile ", OrganizationNodeID: " O_acme ",
		From: "2025-01-01T08:00:00+08:00", To: "2025-01-02T08:00:00+08:00",
		Sort: " repository ", Order: " asc ", Limit: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := request.ActorReferences(); len(got) != 2 || got[0] != "alice" || got[1] != "U_1" {
		t.Fatalf("actor references = %v", got)
	}
	if request.OrganizationNodeID() != "O_acme" || request.FromString() != "2025-01-01T08:00:00+08:00" || request.ToString() != "2025-01-02T08:00:00+08:00" {
		t.Fatalf("parsed request = %+v", request)
	}
	if request.sort.String() != "repository" || request.order.String() != "asc" || request.page.Limit() != 7 {
		t.Fatalf("query modes = %+v", request)
	}
	if len(request.repositories) != 1 || len(request.kinds) != 2 || request.kinds[1].String() != "future_kind" {
		t.Fatalf("parsed filters = repositories %v kinds %v", request.repositories, request.kinds)
	}
}

func TestParseContributionSearchRejectsInvalidBoundaryStates(t *testing.T) {
	t.Parallel()
	validFrom := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	validTo := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	tests := []struct {
		name  string
		input ContributionSearchInput
		want  string
	}{
		{name: "source", input: ContributionSearchInput{Source: "corpus_observation"}, want: "source must be github_profile"},
		{name: "actor", input: ContributionSearchInput{ActorRefs: []string{" "}}, want: "actors[0] is required"},
		{name: "repository", input: ContributionSearchInput{RepositoryRefs: []string{"acme/rocket/extra"}}, want: "repositories[0]"},
		{name: "kind", input: ContributionSearchInput{Kinds: []string{""}}, want: "kinds[0] is required"},
		{name: "from", input: ContributionSearchInput{From: "yesterday"}, want: "from must be RFC 3339"},
		{name: "period", input: ContributionSearchInput{From: validFrom, To: validTo}, want: "to must be after from"},
		{name: "sort", input: ContributionSearchInput{Sort: "newest"}, want: "unsupported contribution sort"},
		{name: "order", input: ContributionSearchInput{Order: "sideways"}, want: "contribution order must be asc or desc"},
		{name: "page", input: ContributionSearchInput{Limit: 101}, want: "contribution search page"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseContributionSearch(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestContributionFilterKeyCannotCollideThroughDelimiters(t *testing.T) {
	t.Parallel()
	first := mustContributionSearch(t, ContributionSearchInput{ActorRefs: []string{"a,b", "c"}})
	second := mustContributionSearch(t, ContributionSearchInput{ActorRefs: []string{"a", "b,c"}})
	if first.filterKey() == second.filterKey() {
		t.Fatal("distinct actor sets produced the same cursor filter key")
	}
}
