package corpus

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// ContributionSearchInput is the transient loose contribution-search
// boundary shape. ParseContributionSearch is its only path into the query
// layer.
type ContributionSearchInput struct {
	ActorRefs          []string
	RepositoryRefs     []string
	Kinds              []string
	Source             string
	OrganizationNodeID string
	From               string
	To                 string
	Sort               string
	Order              string
	Limit              int
	Cursor             string
}

// ContributionSearchRequest is the only executable contribution query
// representation. Every identity, bound, mode, and page has been parsed before
// a corpus read begins. Its private fields keep construction behind
// ParseContributionSearch.
type ContributionSearchRequest struct {
	actorRefs          []actorReference
	repositories       []domain.RepoRef
	kinds              []domain.ContributionKind
	organizationNodeID string
	from               time.Time
	to                 time.Time
	sort               contributionSort
	order              contributionOrder
	page               SearchPage
}

// ParseContributionSearch parses contribution source, identities, time bounds,
// ordering, and paging once at the application boundary.
func ParseContributionSearch(input ContributionSearchInput) (ContributionSearchRequest, error) {
	if source := strings.TrimSpace(input.Source); source != "" && source != "github_profile" {
		return ContributionSearchRequest{}, errors.New("source must be github_profile; corpus_observation is not yet an indexed contribution source")
	}
	actorRefs, err := parseActorReferences(input.ActorRefs)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	repositories, err := parseContributionRepositories(input.RepositoryRefs)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	kinds, err := parseContributionKinds(input.Kinds)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	from, err := parseContributionBound("from", input.From)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	to, err := parseContributionBound("to", input.To)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return ContributionSearchRequest{}, errors.New("to must be after from")
	}
	sortMode, err := parseContributionSort(input.Sort)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	order, err := parseContributionOrder(input.Order)
	if err != nil {
		return ContributionSearchRequest{}, err
	}
	page, err := ParseSearchPage(input.Limit, input.Cursor)
	if err != nil {
		return ContributionSearchRequest{}, fmt.Errorf("contribution search page: %w", err)
	}
	return ContributionSearchRequest{
		actorRefs: actorRefs, repositories: repositories, kinds: kinds,
		organizationNodeID: strings.TrimSpace(input.OrganizationNodeID),
		from:               from, to: to, sort: sortMode, order: order, page: page,
	}, nil
}

// ActorReferences returns the canonical requested actor filters in caller
// order. The returned slice cannot mutate the executable query.
func (r ContributionSearchRequest) ActorReferences() []string {
	refs := make([]string, len(r.actorRefs))
	for i, ref := range r.actorRefs {
		refs[i] = ref.String()
	}
	return refs
}

// OrganizationNodeID returns the optional exact GitHub organization scope.
func (r ContributionSearchRequest) OrganizationNodeID() string { return r.organizationNodeID }

// From returns the optional inclusive contribution bound.
func (r ContributionSearchRequest) From() time.Time { return r.from }

// To returns the optional exclusive contribution bound.
func (r ContributionSearchRequest) To() time.Time { return r.to }

// FromString returns the canonical wire spelling of the inclusive bound.
func (r ContributionSearchRequest) FromString() string { return formatContributionBound(r.from) }

// ToString returns the canonical wire spelling of the exclusive bound.
func (r ContributionSearchRequest) ToString() string { return formatContributionBound(r.to) }

func (r ContributionSearchRequest) filterKey() string {
	actors := r.ActorReferences()
	repositories := make([]string, len(r.repositories))
	for i, ref := range r.repositories {
		repositories[i] = strings.ToLower(ref.String())
	}
	kinds := make([]string, len(r.kinds))
	for i, kind := range r.kinds {
		kinds[i] = kind.String()
	}
	slices.Sort(actors)
	slices.Sort(repositories)
	slices.Sort(kinds)
	canonical, _ := json.Marshal(struct {
		Sort, Order, Organization   string
		Actors, Repositories, Kinds []string
		From, To                    int64
	}{
		Sort: r.sort.String(), Order: r.order.String(), Organization: r.organizationNodeID,
		Actors: actors, Repositories: repositories, Kinds: kinds,
		From: encodeTime(r.from), To: encodeTime(r.to),
	})
	return fmt.Sprintf("%x", sha256.Sum256(canonical))
}

type actorReference string

func parseActorReferences(values []string) ([]actorReference, error) {
	if len(values) > 100 {
		return nil, errors.New("actors are limited to 100 items")
	}
	parsed := make([]actorReference, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("actors[%d] is required", index)
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		parsed = append(parsed, actorReference(value))
	}
	return parsed, nil
}

func (r actorReference) String() string { return string(r) }

func parseContributionRepositories(values []string) ([]domain.RepoRef, error) {
	if len(values) > 100 {
		return nil, errors.New("repositories are limited to 100 items")
	}
	parsed := make([]domain.RepoRef, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		ref, err := domain.ParseRepoRef(value)
		if err != nil {
			return nil, fmt.Errorf("repositories[%d]: %w", index, err)
		}
		key := strings.ToLower(ref.String())
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		parsed = append(parsed, ref)
	}
	return parsed, nil
}

func parseContributionKinds(values []string) ([]domain.ContributionKind, error) {
	if len(values) > 20 {
		return nil, errors.New("kinds are limited to 20 items")
	}
	parsed := make([]domain.ContributionKind, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		kind, err := domain.ParseContributionKind(value)
		if err != nil {
			return nil, fmt.Errorf("kinds[%d] is required: %w", index, err)
		}
		if _, duplicate := seen[kind.String()]; duplicate {
			continue
		}
		seen[kind.String()] = struct{}{}
		parsed = append(parsed, kind)
	}
	return parsed, nil
}

func parseContributionBound(field, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC 3339", field)
	}
	return parsed, nil
}

func formatContributionBound(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

type contributionSort uint8

func parseContributionSort(value string) (contributionSort, error) {
	switch strings.TrimSpace(value) {
	case "", "occurred_at":
		return 0, nil
	case "repository":
		return 1, nil
	case "type":
		return 2, nil
	default:
		return 0, errors.New("unsupported contribution sort")
	}
}

func (s contributionSort) String() string {
	switch s {
	case 1:
		return "repository"
	case 2:
		return "type"
	default:
		return "occurred_at"
	}
}

func (s contributionSort) expression() string {
	switch s {
	case 1:
		return "repository_ref"
	case 2:
		return "contribution_kind"
	default:
		return "occurred_at"
	}
}

type contributionOrder bool

func parseContributionOrder(value string) (contributionOrder, error) {
	switch strings.TrimSpace(value) {
	case "", "desc":
		return false, nil
	case "asc":
		return true, nil
	default:
		return false, errors.New("contribution order must be asc or desc")
	}
}

func (o contributionOrder) String() string {
	if o {
		return "asc"
	}
	return "desc"
}

func (o contributionOrder) sqlDirection() string {
	if o {
		return "ASC"
	}
	return "DESC"
}
