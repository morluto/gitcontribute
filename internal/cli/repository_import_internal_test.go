package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRepositoryImportJSONBoundary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "empty", wantErr: "repository import is empty"},
		{name: "whitespace", input: " \n\t", wantErr: "repository import is empty"},
		{name: "array", input: `["owner/repo", {"owner":"two","repo":"project"}]`, want: []string{"owner/repo", "two/project"}},
		{name: "wrapper", input: `{"repositories":[{"full_name":"owner/repo"}]}`, want: []string{"owner/repo"}},
		{name: "malformed", input: `[`, wantErr: "parse repository import"},
		{name: "empty list", input: `[]`, wantErr: "contains no repositories"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRepositoryImportJSON([]byte(test.input))
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("repositories = %#v, want %#v", got, test.want)
			}
		})
	}
}
