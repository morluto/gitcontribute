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
		return validateSCPLikeRemote(remote)
	}
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
	if at := strings.IndexByte(remote, '@'); at > 0 {
		if strings.Contains(remote[:at], ":") {
			return ErrInvalid
		}
		hostPath := remote[at+1:]
		if colon := strings.IndexByte(hostPath, ':'); colon > 0 && colon < len(hostPath)-1 &&
			!strings.Contains(hostPath[:colon], "@") {
			return nil
		}
	}
	return ErrInvalid
}
