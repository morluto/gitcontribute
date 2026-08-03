package redaction

import "testing"

func TestStringRedactsSupportedCredentialForms(t *testing.T) {
	input := `token="secret-value" Authorization: Bearer abc123 ghp_abcdefghijklmnopqrstuvwxyz1234567890`
	got := String(input)
	for _, secret := range []string{"secret-value", "abc123", "ghp_abcdefghijklmnopqrstuvwxyz1234567890"} {
		if contains(got, secret) {
			t.Errorf("redacted output contains %q: %q", secret, got)
		}
	}
	if got == input {
		t.Fatal("credential input was unchanged")
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
