package contribution

import (
	"strings"
	"testing"
	"unicode/utf8"
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
	EnsureDraftIdentity(&identity, "owner/repo", "pull_request", title, body)
	if identity.TitleBytes != len([]byte(title)) || identity.BodyBytes != len([]byte(body)) {
		t.Fatalf("byte lengths = %d/%d", identity.TitleBytes, identity.BodyBytes)
	}
	if !utf8.ValidString(title) || identity.BodySHA256 == sha256Text(strings.ReplaceAll(body, "\r\n", "\n")) {
		t.Fatal("identity normalized exact bytes")
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
