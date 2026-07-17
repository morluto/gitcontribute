package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zalando/go-keyring"
)

// TokenSource resolves a GitHub authentication token.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// ErrNoToken indicates that a token source could not provide a token.
var ErrNoToken = errors.New("no GitHub token available")

// ErrRequiredToken indicates that an explicitly configured authentication
// source did not provide a token.
var ErrRequiredToken = errors.New("configured GitHub token unavailable")

// KeyringService is the service name used for credentials owned by
// gitcontribute.
const KeyringService = "gitcontribute"

// CommandRunner abstracts process execution so that tests can inject behavior.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// DefaultCommandRunner returns the real command runner.
func DefaultCommandRunner() CommandRunner {
	return execRunner{}
}

// StaticTokenSource returns the provided token if it is non-empty.
func StaticTokenSource(token string) TokenSource {
	return staticTokenSource(token)
}

type staticTokenSource string

func (s staticTokenSource) Token(ctx context.Context) (string, error) {
	if s == "" {
		return "", ErrNoToken
	}
	return string(s), nil
}

// EnvTokenSource resolves a token from an environment variable.
func EnvTokenSource(name string) TokenSource {
	return &envTokenSource{Name: name}
}

type envTokenSource struct {
	Name string
}

func (s *envTokenSource) Token(ctx context.Context) (string, error) {
	v := os.Getenv(s.Name)
	if v == "" {
		return "", ErrNoToken
	}
	return v, nil
}

// KeyringTokenSource resolves a token from the operating system credential
// store. account identifies the credential within the gitcontribute service.
func KeyringTokenSource(account string) TokenSource {
	return &keyringTokenSource{account: account, get: keyring.Get}
}

type keyringTokenSource struct {
	account string
	get     func(service, user string) (string, error)
}

func (s *keyringTokenSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(s.account) == "" {
		return "", ErrNoToken
	}

	token, err := s.get(KeyringService, s.account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", fmt.Errorf("read GitHub token from keyring: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrNoToken
	}
	return token, nil
}

// GhCLITokenSource resolves a token by running `gh auth token`.
// Optional args are passed through to `gh` (for example a `--hostname` flag).
func GhCLITokenSource(runner CommandRunner, args ...string) TokenSource {
	if runner == nil {
		runner = DefaultCommandRunner()
	}
	return &ghTokenSource{runner: runner, args: args}
}

type ghTokenSource struct {
	runner CommandRunner
	args   []string
}

func (s *ghTokenSource) Token(ctx context.Context) (string, error) {
	out, err := s.runner.Run(ctx, "gh", append([]string{"auth", "token"}, s.args...)...)
	out = strings.TrimSpace(out)
	if out == "" {
		return "", ErrNoToken
	}
	if err != nil {
		return "", ErrNoToken
	}
	return out, nil
}

// ChainTokenSource tries each source in order and returns the first non-empty
// token. Sources that return ErrNoToken are skipped.
func ChainTokenSource(sources ...TokenSource) TokenSource {
	return chainTokenSource(sources)
}

type chainTokenSource []TokenSource

func (c chainTokenSource) Token(ctx context.Context) (string, error) {
	for _, s := range c {
		token, err := s.Token(ctx)
		if err == nil && token != "" {
			return token, nil
		}
		if errors.Is(err, ErrNoToken) {
			continue
		}
		if err != nil {
			return "", err
		}
	}
	return "", ErrNoToken
}

// RequireToken prevents an explicitly configured source from silently falling
// back to anonymous GitHub access.
func RequireToken(source TokenSource) TokenSource {
	return requiredTokenSource{source: source}
}

type requiredTokenSource struct {
	source TokenSource
}

func (s requiredTokenSource) Token(ctx context.Context) (string, error) {
	if s.source == nil {
		return "", ErrRequiredToken
	}
	token, err := s.source.Token(ctx)
	if errors.Is(err, ErrNoToken) || (err == nil && token == "") {
		return "", ErrRequiredToken
	}
	return token, err
}

// DefaultEnvToken is the conventional environment variable name for a
// GitHub token.
const DefaultEnvToken = "GITHUB_TOKEN"

// NewTokenSource builds the standard resolution chain: explicit value,
// environment variable, then `gh auth token`.
func NewTokenSource(explicit, envVar string, runner CommandRunner) TokenSource {
	if envVar == "" {
		envVar = DefaultEnvToken
	}
	sources := []TokenSource{StaticTokenSource(explicit), EnvTokenSource(envVar)}
	if runner != nil {
		sources = append(sources, GhCLITokenSource(runner))
	}
	return ChainTokenSource(sources...)
}
