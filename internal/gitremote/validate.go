// Package gitremote validates remote locations accepted by the native Git
// adapters.
package gitremote

import (
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// ErrInvalid indicates that a remote is malformed, unsupported, or embeds
// credentials.
var ErrInvalid = errors.New("invalid Git remote")

// ErrRepositoryIdentity indicates that a valid remote does not encode a
// hosted OWNER/REPOSITORY identity.
var ErrRepositoryIdentity = errors.New("git remote has no hosted repository identity")

// RepositoryIdentity is the credential-free hosted identity encoded by a
// remote URL.
type RepositoryIdentity struct {
	Host  string
	Owner string
	Repo  string
}

// Validate accepts local absolute paths, file URLs, HTTPS URLs, SSH URLs, and
// SCP-like SSH remotes. HTTPS userinfo and SSH passwords are never accepted;
// SSH usernames remain supported because they are not secrets.
func Validate(remote string) error {
	remote = strings.TrimSpace(remote)
	if remote == "" || strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, "\x00\r\n") {
		return ErrInvalid
	}
	if strings.Contains(remote, "::") {
		return ErrInvalid
	}
	if filepath.IsAbs(remote) || path.IsAbs(remote) {
		return nil
	}

	switch {
	case strings.HasPrefix(remote, "file://"):
		return validateFileURL(remote)
	case strings.HasPrefix(remote, "https://"):
		return validateHTTPSURL(remote)
	case strings.HasPrefix(remote, "ssh://"):
		return validateSSHURL(remote)
	default:
		if strings.Contains(remote, "://") {
			return ErrInvalid
		}
		return validateSCPLikeRemote(remote)
	}
}

// ParseRepositoryIdentity extracts a hosted repository identity from an HTTPS,
// SSH URL, or SCP-like SSH remote. Local paths and file URLs are valid remotes
// but do not establish a hosted repository identity.
func ParseRepositoryIdentity(remote string) (RepositoryIdentity, error) {
	remote = strings.TrimSpace(remote)
	if err := Validate(remote); err != nil {
		return RepositoryIdentity{}, err
	}
	if filepath.IsAbs(remote) || path.IsAbs(remote) || strings.HasPrefix(remote, "file://") {
		return RepositoryIdentity{}, ErrRepositoryIdentity
	}

	var host, repoPath string
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "ssh://") {
		u, err := url.Parse(remote)
		if err != nil {
			return RepositoryIdentity{}, ErrRepositoryIdentity
		}
		host, repoPath = u.Hostname(), u.Path
	} else {
		colon := strings.IndexByte(remote, ':')
		host, repoPath = remote[:colon], remote[colon+1:]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
	}
	parts := strings.Split(strings.Trim(repoPath, "/"), "/")
	if host == "" || len(parts) != 2 {
		return RepositoryIdentity{}, ErrRepositoryIdentity
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	if parts[0] == "" || repo == "" {
		return RepositoryIdentity{}, ErrRepositoryIdentity
	}
	return RepositoryIdentity{Host: strings.ToLower(host), Owner: parts[0], Repo: repo}, nil
}

func validateFileURL(remote string) error {
	u, err := url.Parse(remote)
	if err != nil || u.User != nil {
		return ErrInvalid
	}
	return nil
}

func validateHTTPSURL(remote string) error {
	u, err := url.Parse(remote)
	if err != nil || u.User != nil || u.Host == "" {
		return ErrInvalid
	}
	return nil
}

func validateSSHURL(remote string) error {
	u, err := url.Parse(remote)
	if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
		return ErrInvalid
	}
	if u.User == nil {
		return nil
	}
	if u.User.Username() == "" {
		return ErrInvalid
	}
	if _, ok := u.User.Password(); ok {
		return ErrInvalid
	}
	return nil
}

func validateSCPLikeRemote(remote string) error {
	colon := strings.IndexByte(remote, ':')
	if colon <= 0 || colon == len(remote)-1 {
		return ErrInvalid
	}
	if at := strings.IndexByte(remote, '@'); at > colon {
		return ErrInvalid
	}
	host := remote[:colon]
	if strings.ContainsAny(host, "/\\ \t") || strings.Count(host, "@") > 1 {
		return ErrInvalid
	}
	if at := strings.IndexByte(host, '@'); at == 0 || at == len(host)-1 {
		return ErrInvalid
	}
	return nil
}
