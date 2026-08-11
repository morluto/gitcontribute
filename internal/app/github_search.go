package app

import (
	"errors"
	"strings"
)

type githubSearchOrder uint8

const (
	githubSearchOrderUnspecified githubSearchOrder = iota
	githubSearchAscending
	githubSearchDescending
)

func parseGitHubSearchOrder(value string, fallback githubSearchOrder) (githubSearchOrder, error) {
	switch strings.TrimSpace(value) {
	case "":
		return fallback, nil
	case "asc":
		return githubSearchAscending, nil
	case "desc":
		return githubSearchDescending, nil
	default:
		return 0, errors.New("order must be asc or desc")
	}
}

func (o githubSearchOrder) String() string {
	switch o {
	case githubSearchAscending:
		return "asc"
	case githubSearchDescending:
		return "desc"
	default:
		return ""
	}
}

type githubSearchPage struct {
	number int
	limit  int
}

type githubSearchPageProblem uint8

const (
	githubSearchPageValid githubSearchPageProblem = iota
	githubSearchLimitInvalid
	githubSearchPageInvalid
)

func parseGitHubSearchPage(limit, page int) (githubSearchPage, githubSearchPageProblem) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return githubSearchPage{}, githubSearchLimitInvalid
	}
	if page == 0 {
		page = 1
	}
	if page < 1 || page > 1000 || (page-1)*limit >= 1000 {
		return githubSearchPage{}, githubSearchPageInvalid
	}
	return githubSearchPage{number: page, limit: limit}, githubSearchPageValid
}
