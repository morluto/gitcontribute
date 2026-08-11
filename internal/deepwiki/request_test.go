package deepwiki

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRequestConstructsConcreteOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		action       string
		repository   string
		repositories []string
		question     string
		wantAction   Action
		wantRepos    []string
		wantQuestion string
	}{
		{name: "structure", action: " structure ", repository: " owner/repo ", wantAction: Structure, wantRepos: []string{"owner/repo"}},
		{name: "contents", action: "contents", repository: "owner/repo", wantAction: Contents, wantRepos: []string{"owner/repo"}},
		{name: "question", action: "question", repositories: []string{" one/repo ", "two/repo"}, question: " Compare them. ", wantAction: Question, wantRepos: []string{"one/repo", "two/repo"}, wantQuestion: "Compare them."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request, err := ParseRequest(tt.action, tt.repository, tt.repositories, tt.question)
			if err != nil {
				t.Fatal(err)
			}
			if request.Action() != tt.wantAction || !reflect.DeepEqual(request.Repositories(), tt.wantRepos) || request.Question() != tt.wantQuestion {
				t.Fatalf("request = (%s, %v, %q), want (%s, %v, %q)", request.Action(), request.Repositories(), request.Question(), tt.wantAction, tt.wantRepos, tt.wantQuestion)
			}
		})
	}
}

func TestParseRequestRejectsContradictoryAndMalformedInputs(t *testing.T) {
	t.Parallel()
	tooMany := make([]string, MaxRepositories+1)
	for i := range tooMany {
		tooMany[i] = "owner/repo"
	}
	tests := []struct {
		name         string
		action       string
		repository   string
		repositories []string
		question     string
	}{
		{name: "unknown action", action: "unknown"},
		{name: "missing repository", action: "structure"},
		{name: "malformed repository", action: "contents", repository: "owner/repo/extra"},
		{name: "repository mode with question fields", action: "structure", repository: "owner/repo", repositories: []string{"other/repo"}},
		{name: "question with repository field", action: "question", repository: "owner/repo", repositories: []string{"other/repo"}, question: "why"},
		{name: "question without repositories", action: "question", question: "why"},
		{name: "question without text", action: "question", repositories: []string{"owner/repo"}, question: "  "},
		{name: "question with malformed repository", action: "question", repositories: []string{"owner/repo", "bad"}, question: "why"},
		{name: "too many repositories", action: "question", repositories: tooMany, question: "why"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if request, err := ParseRequest(tt.action, tt.repository, tt.repositories, tt.question); err == nil {
				t.Fatalf("ParseRequest returned %v", request)
			}
		})
	}
}

func TestRequestRepositoryProjectionDoesNotExposeMutableState(t *testing.T) {
	t.Parallel()
	request := mustParseRequest(t, "question", "", []string{"one/repo", "two/repo"}, "Compare")
	repositories := request.Repositories()
	repositories[0] = strings.Repeat("x", 20)
	if got := request.Repositories(); !reflect.DeepEqual(got, []string{"one/repo", "two/repo"}) {
		t.Fatalf("request repositories mutated through projection: %v", got)
	}
}
