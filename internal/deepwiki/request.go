package deepwiki

import (
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

// MaxRepositories is the provider-supported bound for one cross-repository
// question.
const MaxRepositories = 10

// Action identifies the concrete DeepWiki read represented by a Request.
type Action uint8

const (
	Structure Action = iota + 1
	Contents
	Question
)

// String returns the protocol spelling for the action.
func (a Action) String() string {
	switch a {
	case Structure:
		return "structure"
	case Contents:
		return "contents"
	case Question:
		return "question"
	default:
		return ""
	}
}

// Request is a parsed DeepWiki read. Its implementations are sealed so
// repository reads cannot carry question-only fields and questions cannot
// carry the single-repository representation.
type Request interface {
	Action() Action
	Repositories() []string
	Question() string
	isRequest()
}

type repositoryRequest struct {
	action     Action
	repository domain.RepoRef
}

func (r repositoryRequest) Action() Action         { return r.action }
func (r repositoryRequest) Repositories() []string { return []string{r.repository.String()} }
func (repositoryRequest) Question() string         { return "" }
func (repositoryRequest) isRequest()               {}

type questionRequest struct {
	repositories []domain.RepoRef
	question     string
}

func (questionRequest) Action() Action { return Question }
func (r questionRequest) Repositories() []string {
	values := make([]string, len(r.repositories))
	for i, repository := range r.repositories {
		values[i] = repository.String()
	}
	return values
}
func (r questionRequest) Question() string { return r.question }
func (questionRequest) isRequest()         {}

// ParseRequest turns the loose protocol fields into one concrete DeepWiki
// operation. Successful callers pass the returned Request inward instead of
// retaining or revalidating the original field bag.
func ParseRequest(action, repository string, repositories []string, question string) (Request, error) {
	switch strings.TrimSpace(action) {
	case "structure":
		return parseRepositoryRequest(Structure, repository, repositories, question)
	case "contents":
		return parseRepositoryRequest(Contents, repository, repositories, question)
	case "question":
		return parseQuestionRequest(repository, repositories, question)
	default:
		return nil, errors.New("action must be structure, contents, or question")
	}
}

func parseRepositoryRequest(action Action, repository string, repositories []string, question string) (Request, error) {
	if len(repositories) > 0 || strings.TrimSpace(question) != "" {
		return nil, fmt.Errorf("%s accepts repository only", action)
	}
	parsed, err := domain.ParseRepoRef(repository)
	if err != nil {
		return nil, fmt.Errorf("invalid DeepWiki repository: %w", err)
	}
	return repositoryRequest{action: action, repository: parsed}, nil
}

func parseQuestionRequest(repository string, repositories []string, question string) (Request, error) {
	if strings.TrimSpace(repository) != "" {
		return nil, errors.New("question accepts repositories only")
	}
	if len(repositories) == 0 {
		return nil, errors.New("repositories are required for question")
	}
	if len(repositories) > MaxRepositories {
		return nil, fmt.Errorf("DeepWiki supports at most %d repositories", MaxRepositories)
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("question is required")
	}
	parsed := make([]domain.RepoRef, len(repositories))
	for i, repository := range repositories {
		ref, err := domain.ParseRepoRef(repository)
		if err != nil {
			return nil, fmt.Errorf("invalid DeepWiki repository at index %d: %w", i, err)
		}
		parsed[i] = ref
	}
	return questionRequest{repositories: parsed, question: question}, nil
}
