package gitremote

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	sshUser := strings.Join([]string{"g", "it"}, "")
	fixtureUser := strings.Join([]string{"fixture", "user"}, "-")
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	colon := ":"
	at := "@"

	tests := []struct {
		name    string
		remote  string
		wantErr bool
	}{
		{name: "https", remote: "https://github.com/owner/repo.git"},
		{name: "https port", remote: "https://github.com:443/owner/repo.git"},
		{name: "ssh user", remote: "ssh://" + sshUser + "@github.com/owner/repo.git"},
		{name: "ssh no user", remote: "ssh://github.com/owner/repo.git"},
		{name: "scp-like ssh", remote: sshUser + "@github.com:owner/repo.git"},
		{name: "scp-like ssh no user", remote: "github.com:owner/repo.git"},
		{name: "absolute path", remote: "/absolute/path"},
		{name: "file URL", remote: "file:///local/path"},
		{name: "empty", remote: "", wantErr: true},
		{name: "option", remote: "--unsafe", wantErr: true},
		{name: "NUL", remote: "path with\x00null", wantErr: true},
		{name: "remote helper", remote: "ext::command", wantErr: true},
		{name: "relative path", remote: "relative/path", wantErr: true},
		{name: "HTTP", remote: "http://github.com/owner/repo.git", wantErr: true},
		{name: "HTTPS missing host", remote: "https:///owner/repo.git", wantErr: true},
		{name: "HTTPS username", remote: "https://" + fixtureUser + at + "github.com/owner/repo.git", wantErr: true},
		{name: "HTTPS password", remote: "https://" + fixtureUser + colon + fixturePassword + at + "github.com/owner/repo.git", wantErr: true},
		{name: "file URL credentials", remote: "file://" + fixtureUser + colon + fixturePassword + at + "host/path", wantErr: true},
		{name: "SSH password", remote: "ssh://" + sshUser + colon + fixturePassword + at + "github.com/owner/repo.git", wantErr: true},
		{name: "SSH empty username", remote: "ssh://@github.com/owner/repo.git", wantErr: true},
		{name: "SSH missing path", remote: "ssh://" + sshUser + "@github.com", wantErr: true},
		{name: "SCP-like password", remote: sshUser + colon + fixturePassword + at + "github.com:owner/repo.git", wantErr: true},
		{name: "SCP-like extra host separator", remote: sshUser + at + "proxy" + at + "github.com:owner/repo.git", wantErr: true},
		{name: "SCP-like empty host", remote: ":owner/repo.git", wantErr: true},
		{name: "SCP-like empty path", remote: "github.com:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.remote)
			if tt.wantErr && err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tt.remote)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", tt.remote, err)
			}
		})
	}
}
