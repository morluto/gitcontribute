package tracking

import (
	"strings"
	"testing"
)

func TestSanitizeMetadataRedactsSensitiveKeysRecursively(t *testing.T) {
	in := map[string]any{
		"title":       "ok",
		"token":       "secret-token",
		"api_key":     "secret-key",
		"authToken":   "secret-auth",
		"client_id":   "secret-client",
		"private_key": "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----",
		"nested": map[string]any{
			"password": "nested-password",
			"normal":   "kept",
			"deeper": map[string]any{
				"github_pat": "secret-pat",
			},
		},
		"list": []any{
			map[string]any{"secret": "list-secret"},
			"plain string",
		},
		"public": "visible",
	}

	out := sanitizeMetadata(in)

	if out["title"] != "ok" {
		t.Fatalf("title = %v, want ok", out["title"])
	}
	if out["public"] != "visible" {
		t.Fatalf("public = %v, want visible", out["public"])
	}

	sensitiveKeys := []string{"token", "api_key", "authToken", "client_id", "private_key"}
	for _, k := range sensitiveKeys {
		if out[k] != "[REDACTED]" {
			t.Fatalf("%s = %v, want [REDACTED]", k, out[k])
		}
	}

	nested := out["nested"].(map[string]any)
	if nested["password"] != "[REDACTED]" {
		t.Fatalf("nested.password = %v, want [REDACTED]", nested["password"])
	}
	if nested["normal"] != "kept" {
		t.Fatalf("nested.normal = %v, want kept", nested["normal"])
	}
	deeper := nested["deeper"].(map[string]any)
	if deeper["github_pat"] != "[REDACTED]" {
		t.Fatalf("nested.deeper.github_pat = %v, want [REDACTED]", deeper["github_pat"])
	}

	list := out["list"].([]any)
	item := list[0].(map[string]any)
	if item["secret"] != "[REDACTED]" {
		t.Fatalf("list[0].secret = %v, want [REDACTED]", item["secret"])
	}
	if list[1] != "plain string" {
		t.Fatalf("list[1] = %v, want plain string", list[1])
	}
}

func TestSanitizeMetadataRedactsValueBasedSecrets(t *testing.T) {
	in := map[string]any{
		"note":       "my token is ghp_abcdef012345678901234567890123456789",
		"body":       "Authorization: Bearer super-secret",
		"path":       "/home/user/.ssh/id_rsa",
		"unaffected": "keep this",
	}

	out := sanitizeMetadata(in)

	if got := out["note"].(string); !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("note not redacted: %q", got)
	}
	if got := out["body"].(string); strings.Contains(got, "super-secret") {
		t.Fatalf("body leaked secret: %q", got)
	}
	if got := out["path"].(string); !strings.Contains(got, "[REDACTED_PATH]") {
		t.Fatalf("path not redacted: %q", got)
	}
	if out["unaffected"] != "keep this" {
		t.Fatalf("unaffected = %v, want keep this", out["unaffected"])
	}
}

func TestSanitizeMetadataHandlesNilAndScalars(t *testing.T) {
	if out := sanitizeMetadata(nil); out != nil {
		t.Fatalf("sanitizeMetadata(nil) = %v, want nil", out)
	}

	in := map[string]any{
		"count": 42,
		"ok":    true,
		"empty": "",
	}
	out := sanitizeMetadata(in)
	if out["count"] != 42 || out["ok"] != true || out["empty"] != "" {
		t.Fatalf("scalar values altered: %+v", out)
	}
}
