package discovery

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

// ParseRepoRef converts an explicit owner/repo reference into a validated
// domain.RepoRef. Supported forms are:
//
//   - owner/repo
//   - github.com/owner/repo
//   - https://github.com/owner/repo
//   - http://github.com/owner/repo
//   - git@github.com:owner/repo
//   - ssh://git@github.com/owner/repo
//
// A trailing ".git" and query/fragment components are stripped.
func ParseRepoRef(s string) (domain.RepoRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.RepoRef{}, fmt.Errorf("empty repo reference")
	}

	s = stripURLScheme(s)

	// Handle the common ssh://git@github.com/owner/repo form through url.Parse,
	// and the git@github.com:path form through string manipulation above.
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return domain.RepoRef{}, fmt.Errorf("invalid repo URL %q: %w", s, err)
		}
		if !isGitHubHost(u.Host) {
			return domain.RepoRef{}, fmt.Errorf("unsupported host %q", u.Host)
		}
		s = u.Path
	}

	if strings.HasPrefix(strings.ToLower(s), "github.com/") {
		s = s[len("github.com/"):]
	}

	s = strings.Trim(s, "/")
	if strings.HasSuffix(strings.ToLower(s), ".git") {
		s = s[:len(s)-len(".git")]
	}

	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.RepoRef{}, fmt.Errorf("invalid repo reference %q: expected owner/repo", s)
	}

	ref := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	if err := ref.Validate(); err != nil {
		return domain.RepoRef{}, err
	}
	return ref, nil
}

// ParseRepoRefs parses a slice of explicit references. It stops at the first
// invalid entry and returns the successfully parsed prefix.
func ParseRepoRefs(inputs []string) ([]domain.RepoRef, error) {
	refs := make([]domain.RepoRef, 0, len(inputs))
	for _, input := range inputs {
		ref, err := ParseRepoRef(input)
		if err != nil {
			return refs, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func stripURLScheme(s string) string {
	// git@github.com:owner/repo is not a URL and should not be url.Parse'd
	// directly. Treat it as a host:path pair first.
	if idx := strings.Index(s, "@"); idx > 0 {
		hostPath := s[idx+1:]
		colon := strings.Index(hostPath, ":")
		if colon > 0 {
			host := hostPath[:colon]
			path := hostPath[colon+1:]
			if isGitHubHost(host) && !strings.Contains(path, "://") {
				return path
			}
		}
	}
	return s
}

func isGitHubHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "github.com"
}
