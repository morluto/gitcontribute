package corpus

import (
	"errors"
	"fmt"
	"strings"
)

// ActorSearchInput is the transient loose actor-search boundary shape.
type ActorSearchInput struct {
	Query  string
	Kinds  []string
	Sort   string
	Limit  int
	Cursor string
}

// ActorSearchRequest is the only executable actor query representation. Its
// private fields keep construction behind ParseActorSearch.
type ActorSearchRequest struct {
	query string
	kinds actorKindSet
	sort  actorSort
	page  SearchPage
}

// ParseActorSearch parses actor kinds, ordering, and paging once before a
// corpus read begins.
func ParseActorSearch(input ActorSearchInput) (ActorSearchRequest, error) {
	page, err := ParseSearchPage(input.Limit, input.Cursor)
	if err != nil {
		return ActorSearchRequest{}, fmt.Errorf("actor search page: %w", err)
	}
	kinds, err := parseActorKindSet(input.Kinds)
	if err != nil {
		return ActorSearchRequest{}, err
	}
	sortMode, err := parseActorSort(input.Sort)
	if err != nil {
		return ActorSearchRequest{}, err
	}
	return ActorSearchRequest{query: strings.TrimSpace(input.Query), kinds: kinds, sort: sortMode, page: page}, nil
}

type actorKindSet uint8

func parseActorKindSet(values []string) (actorKindSet, error) {
	if len(values) > 5 {
		return 0, errors.New("actor search kinds cannot exceed 5 items")
	}
	var set actorKindSet
	for _, value := range values {
		var bit actorKindSet
		switch strings.TrimSpace(value) {
		case "user":
			bit = 1 << 0
		case "bot":
			bit = 1 << 1
		case "organization":
			bit = 1 << 2
		case "mannequin":
			bit = 1 << 3
		case "unknown":
			bit = 1 << 4
		default:
			return 0, fmt.Errorf("unsupported actor kind %q", value)
		}
		if set&bit != 0 {
			return 0, fmt.Errorf("duplicate actor kind %q", value)
		}
		set |= bit
	}
	return set, nil
}

func (s actorKindSet) values() []string {
	out := make([]string, 0, 5)
	for index, value := range []string{"user", "bot", "organization", "mannequin", "unknown"} {
		if s&(1<<index) != 0 {
			out = append(out, value)
		}
	}
	return out
}

func (s actorKindSet) key() string { return strings.Join(s.values(), ",") }

type actorSort uint8

func parseActorSort(value string) (actorSort, error) {
	switch strings.TrimSpace(value) {
	case "", "relevance":
		return 0, nil
	case "login":
		return 1, nil
	case "followers":
		return 2, nil
	case "public_repositories":
		return 3, nil
	case "profile_updated_at":
		return 4, nil
	case "observed_at":
		return 5, nil
	default:
		return 0, errors.New("unsupported actor sort")
	}
}

func (s actorSort) String() string {
	switch s {
	case 1:
		return "login"
	case 2:
		return "followers"
	case 3:
		return "public_repositories"
	case 4:
		return "profile_updated_at"
	case 5:
		return "observed_at"
	default:
		return "relevance"
	}
}

func (s actorSort) expression(rank string) string {
	switch s {
	case 1:
		return `a.current_login COLLATE NOCASE, a.id`
	case 2:
		return `COALESCE(p.followers,-1) DESC, a.id`
	case 3:
		return `COALESCE(p.public_repositories,-1) DESC, a.id`
	case 4:
		return `COALESCE(p.source_updated_at,0) DESC, a.id`
	case 5:
		return `COALESCE(p.observed_at,0) DESC, a.id`
	default:
		return rank + `, a.source_updated_at DESC, a.id`
	}
}
