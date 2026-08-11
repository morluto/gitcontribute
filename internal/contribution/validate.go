package contribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var (
	templatePlaceholder = regexp.MustCompile(`(?i)(\{\{[^{}\n]+\}\}|<!--\s*(?:TODO|FILL|REPLACE)[\s\S]*?-->)`)
	closingCandidate    = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\b[^\n]*#`)
	validClosing        = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:[\w.-]+/[\w.-]+)?#\d+\b`)
)

func populateDraftIdentity(identity *DraftIdentity, repo string, kind domain.ThreadKind, title, body string, all []*evidence.Evidence) {
	identity.ID = uuid.NewString()
	identity.Repository = repo
	identity.Kind = kind
	identity.TitleBytes = len([]byte(title))
	identity.BodyBytes = len([]byte(body))
	identity.TitleSHA256 = sha256Text(title)
	identity.BodySHA256 = sha256Text(body)
	for _, item := range all {
		if item != nil {
			identity.EvidenceIDs = append(identity.EvidenceIDs, item.ID)
		}
	}
	slices.Sort(identity.EvidenceIDs)
	identity.EvidenceIDs = slices.Compact(identity.EvidenceIDs)
	identity.Warnings = ValidateDraftBytes([]byte(title), []byte(body))
}

// EnsureDraftIdentity binds exact bytes for callers that construct drafts
// directly rather than through Renderer.
func EnsureDraftIdentity(identity *DraftIdentity, repo string, kind domain.ThreadKind, title, body string) {
	if identity.ID != "" {
		return
	}
	populateDraftIdentity(identity, repo, kind, title, body, nil)
}

// ParseStored validates and canonicalizes a persisted issue draft.
func (d *IssueDraft) ParseStored() error {
	if d == nil {
		return fmt.Errorf("issue draft is required")
	}
	return d.parseStored(domain.IssueKind, d.Title, d.Body)
}

// ParseStored validates and canonicalizes a persisted pull-request draft.
func (d *PullRequestDraft) ParseStored() error {
	if d == nil {
		return fmt.Errorf("pull request draft is required")
	}
	return d.parseStored(domain.PullRequestKind, d.Title, d.Body)
}

func (d *DraftIdentity) parseStored(expected domain.ThreadKind, title, body string) error {
	if d.ID == "" || d.Revision < 1 {
		return fmt.Errorf("draft id and positive revision are required")
	}
	kind, err := domain.ParseThreadKind(string(d.Kind))
	if err != nil {
		return err
	}
	if kind != expected {
		return fmt.Errorf("stored %s draft has kind %q", expected, d.Kind)
	}
	d.Kind = kind
	if d.TitleBytes != len([]byte(title)) || d.BodyBytes != len([]byte(body)) ||
		d.TitleSHA256 != sha256Text(title) || d.BodySHA256 != sha256Text(body) {
		return fmt.Errorf("stored draft byte identity does not match its content")
	}
	for index := range d.Warnings {
		severity, err := ParseDraftDiagnosticSeverity(string(d.Warnings[index].Severity))
		if err != nil {
			return err
		}
		d.Warnings[index].Severity = severity
	}
	return nil
}

// ParseDraftDiagnosticSeverity parses a durable draft diagnostic severity.
func ParseDraftDiagnosticSeverity(value string) (DraftDiagnosticSeverity, error) {
	switch DraftDiagnosticSeverity(value) {
	case DraftDiagnosticError:
		return DraftDiagnosticError, nil
	case DraftDiagnosticWarning:
		return DraftDiagnosticWarning, nil
	default:
		return "", fmt.Errorf("unsupported draft diagnostic severity %q", value)
	}
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// ValidateDraftBytes inspects the final bytes and never normalizes or rewrites
// them. Markdown prose is identified through Goldmark's source segments.
func ValidateDraftBytes(title, body []byte) []DraftDiagnostic {
	var out []DraftDiagnostic
	if !utf8.Valid(title) || !utf8.Valid(body) {
		return []DraftDiagnostic{{Code: "invalid_utf8", Severity: DraftDiagnosticError, Message: "title and body must be valid UTF-8"}}
	}
	document := goldmark.DefaultParser().Parse(text.NewReader(body))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindText {
			return ast.WalkContinue, nil
		}
		textNode, ok := node.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		segment := textNode.Segment
		value := segment.Value(body)
		if at := bytes.Index(value, []byte(`\n`)); at >= 0 {
			out = append(out, DraftDiagnostic{
				Code: "literal_escaped_newline", Severity: DraftDiagnosticWarning,
				Message: "literal \\\\n appears in a prose region", ByteOffset: segment.Start + at,
			})
		}
		return ast.WalkContinue, nil
	})
	if offset := unmatchedFenceOffset(body); offset >= 0 {
		out = append(out, DraftDiagnostic{Code: "unterminated_fence", Severity: DraftDiagnosticError, Message: "fenced code block is unterminated", ByteOffset: offset})
	}
	if match := templatePlaceholder.FindIndex(body); match != nil {
		out = append(out, DraftDiagnostic{Code: "unresolved_placeholder", Severity: DraftDiagnosticError, Message: "unresolved template placeholder", ByteOffset: match[0]})
	}
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if closingCandidate.Match(line) && !validClosing.Match(line) {
			offset := bytes.Index(body, line)
			out = append(out, DraftDiagnostic{Code: "malformed_closing_reference", Severity: DraftDiagnosticError, Message: "malformed GitHub closing reference", ByteOffset: offset})
			break
		}
	}
	return out
}

// ValidateRequiredTemplateSections reports headings declared by repository
// guidance but absent from the rendered contribution before its guidance
// appendix.
func ValidateRequiredTemplateSections(body, guidance []byte) []DraftDiagnostic {
	required := markdownHeadings(guidance)
	if len(required) == 0 {
		return nil
	}
	contributionBody := body
	if index := bytes.Index(body, []byte("\n## Repository Guidance\n")); index >= 0 {
		contributionBody = body[:index]
	}
	present := markdownHeadings(contributionBody)
	var out []DraftDiagnostic
	for heading := range required {
		if _, ok := present[heading]; !ok {
			out = append(out, DraftDiagnostic{
				Code: "required_template_section_missing", Severity: DraftDiagnosticError,
				Message: "required repository template section is absent: " + heading,
			})
		}
	}
	return out
}

func markdownHeadings(source []byte) map[string]struct{} {
	out := map[string]struct{}{}
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == ast.KindHeading {
			heading := strings.ToLower(strings.TrimSpace(markdownNodeText(node, source)))
			if heading != "" {
				out[heading] = struct{}{}
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

func markdownNodeText(root ast.Node, source []byte) string {
	var value strings.Builder
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Text:
			value.Write(node.Value(source))
		case *ast.String:
			value.Write(node.Value)
		}
		return ast.WalkContinue, nil
	})
	return value.String()
}

func unmatchedFenceOffset(body []byte) int {
	lines := strings.Split(string(body), "\n")
	openMarker, openOffset, offset := "", -1, 0
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) <= 3 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			marker := strings.Repeat(string(trimmed[0]), leadingRun(trimmed, trimmed[0]))
			if len(marker) >= 3 {
				if openMarker == "" {
					openMarker, openOffset = marker, offset+len(line)-len(trimmed)
				} else if trimmed[0] == openMarker[0] && len(marker) >= len(openMarker) {
					openMarker, openOffset = "", -1
				}
			}
		}
		offset += len(line) + 1
	}
	return openOffset
}

func leadingRun(value string, target byte) int {
	for i := 0; i < len(value); i++ {
		if value[i] != target {
			return i
		}
	}
	return len(value)
}
