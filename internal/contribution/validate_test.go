package contribution

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestValidateDraftBytesUnderstandsMarkdownSourceRegions(t *testing.T) {
	body := []byte("## Proof\n\nReal line\nnext\n\n```text\nliteral \\\\n is data\n```\n")
	if findings := ValidateDraftBytes([]byte("Unicode ✓"), body); len(findings) != 0 {
		t.Fatalf("valid markdown findings = %+v", findings)
	}
	for name, body := range map[string]string{
		"literal escaped newline": "prose has \\\\n here",
		"unmatched fence":         "```go\nfmt.Println()\n",
		"placeholder":             "## Test\n\n{{ fill_me }}",
		"closing reference":       "Fixes #abc",
	} {
		t.Run(name, func(t *testing.T) {
			if findings := ValidateDraftBytes([]byte("title"), []byte(body)); len(findings) == 0 {
				t.Fatal("expected finding")
			}
		})
	}
}

func TestDraftIdentityUsesExactUnicodeAndCRLFBytes(t *testing.T) {
	identity := DraftIdentity{}
	title, body := "Fix ✓", "a\r\nb\n"
	EnsureDraftIdentity(&identity, "owner/repo", domain.PullRequestKind, title, body)
	if identity.TitleBytes != len([]byte(title)) || identity.BodyBytes != len([]byte(body)) {
		t.Fatalf("byte lengths = %d/%d", identity.TitleBytes, identity.BodyBytes)
	}
	if !utf8.ValidString(title) || identity.BodySHA256 == sha256Text(strings.ReplaceAll(body, "\r\n", "\n")) {
		t.Fatal("identity normalized exact bytes")
	}
}

func TestStoredDraftRejectsInvalidIdentityAndDiagnosticSeverity(t *testing.T) {
	newDraft := func() *IssueDraft {
		draft := &IssueDraft{OpportunityID: "opp", Title: "title", Body: "body"}
		EnsureDraftIdentity(&draft.DraftIdentity, "owner/repo", domain.IssueKind, draft.Title, draft.Body)
		draft.Revision = 1
		return draft
	}
	tests := []struct {
		name   string
		mutate func(*IssueDraft)
	}{
		{name: "wrong kind", mutate: func(draft *IssueDraft) { draft.Kind = domain.PullRequestKind }},
		{name: "changed bytes", mutate: func(draft *IssueDraft) { draft.Body = "changed" }},
		{name: "unknown severity", mutate: func(draft *IssueDraft) {
			draft.Warnings = []DraftDiagnostic{{Code: "finding", Severity: "notice", Message: "message"}}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			draft := newDraft()
			testCase.mutate(draft)
			if err := draft.ParseStored(); err == nil {
				t.Fatal("invalid stored draft was accepted")
			}
		})
	}
}

func TestValidateRequiredTemplateSectionsDetectsChangedTemplate(t *testing.T) {
	guidance := []byte("## Test plan\n\nRequired.\n\n## Compatibility\n")
	body := []byte("## Compatibility\n\nNo changes.\n\n## Repository Guidance\n\n" + string(guidance))
	findings := ValidateRequiredTemplateSections(body, guidance)
	if len(findings) != 1 || findings[0].Code != "required_template_section_missing" || !strings.Contains(findings[0].Message, "test plan") {
		t.Fatalf("findings = %+v", findings)
	}
}
