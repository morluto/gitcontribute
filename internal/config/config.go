package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config is the typed TOML application configuration. It never stores raw
// GitHub tokens; only a token source descriptor.
type Config struct {
	Database    string      `toml:"database,omitempty"`
	TokenSource TokenSource `toml:"token_source,omitempty"`
	Crawl       Crawl       `toml:"crawl,omitempty"`
	Output      Output      `toml:"output,omitempty"`
}

// TokenSource describes how to obtain a GitHub token. The token itself is never
// persisted here.
type TokenSource struct {
	Method string `toml:"method"`
	Key    string `toml:"key,omitempty"`
}

// Crawl holds crawl budgets and concurrency limits.
type Crawl struct {
	Budget      int    `toml:"budget"`
	Concurrency int    `toml:"concurrency"`
	RetryLimit  int    `toml:"retry_limit"`
	Timeout     string `toml:"timeout,omitempty"`
}

// Output holds default output settings.
type Output struct {
	Format     string `toml:"format"`
	MaxResults int    `toml:"max_results"`
}

// Default returns a Config populated with built-in defaults. Database and paths
// are left empty so that ApplyDefaults can resolve them against Paths.
func Default() *Config {
	return &Config{
		TokenSource: TokenSource{Method: "none"},
		Crawl: Crawl{
			Budget:      1000,
			Concurrency: 4,
			RetryLimit:  3,
			Timeout:     "30s",
		},
		Output: Output{
			Format:     "text",
			MaxResults: 100,
		},
	}
}

// Load reads TOML from r and decodes it into a Config. Unknown fields are
// rejected when go-toml/v2's strict mode is available.
func Load(r io.Reader) (*Config, error) {
	var cfg Config
	dec := toml.NewDecoder(r).DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &cfg, nil
}

// LoadFile reads and decodes the TOML file at path.
func LoadFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}

// Save writes cfg to path atomically with 0600 permissions.
func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("cannot save nil config")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".gitcontribute-config-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		return fmt.Errorf("chmod temp config file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp config file: %w", err)
	}
	cleanup = false
	return nil
}

// ApplyDefaults fills zero-valued fields with defaults and resolves the
// database path against paths when it is unset.
func ApplyDefaults(cfg *Config, paths *Paths) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if cfg.TokenSource.Method == "" {
		cfg.TokenSource.Method = "none"
	}
	if cfg.Crawl.Budget == 0 {
		cfg.Crawl.Budget = 1000
	}
	if cfg.Crawl.Concurrency == 0 {
		cfg.Crawl.Concurrency = 4
	}
	if cfg.Crawl.RetryLimit == 0 {
		cfg.Crawl.RetryLimit = 3
	}
	if cfg.Crawl.Timeout == "" {
		cfg.Crawl.Timeout = "30s"
	}
	if cfg.Output.Format == "" {
		cfg.Output.Format = "text"
	}
	if cfg.Output.MaxResults == 0 {
		cfg.Output.MaxResults = 100
	}

	if cfg.Database == "" && paths != nil {
		db, err := paths.DatabasePath()
		if err != nil {
			return fmt.Errorf("resolve default database path: %w", err)
		}
		cfg.Database = db
	}
	return nil
}

// ApplyEnv overrides cfg fields from environment variables. The getter may be
// os.Getenv or an injected map for tests.
func ApplyEnv(cfg *Config, getenv func(string) string) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if getenv == nil {
		return nil
	}

	if v := getenv("GITCONTRIBUTE_DATABASE"); v != "" {
		cfg.Database = v
	}
	if v := getenv("GITCONTRIBUTE_TOKEN_SOURCE_METHOD"); v != "" {
		cfg.TokenSource.Method = strings.ToLower(v)
	}
	if v := getenv("GITCONTRIBUTE_TOKEN_SOURCE_KEY"); v != "" {
		cfg.TokenSource.Key = v
	}
	if v := getenv("GITCONTRIBUTE_CRAWL_BUDGET"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("GITCONTRIBUTE_CRAWL_BUDGET: %w", err)
		}
		cfg.Crawl.Budget = n
	}
	if v := getenv("GITCONTRIBUTE_CRAWL_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("GITCONTRIBUTE_CRAWL_CONCURRENCY: %w", err)
		}
		cfg.Crawl.Concurrency = n
	}
	if v := getenv("GITCONTRIBUTE_CRAWL_RETRY_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("GITCONTRIBUTE_CRAWL_RETRY_LIMIT: %w", err)
		}
		cfg.Crawl.RetryLimit = n
	}
	if v := getenv("GITCONTRIBUTE_CRAWL_TIMEOUT"); v != "" {
		cfg.Crawl.Timeout = v
	}
	if v := getenv("GITCONTRIBUTE_OUTPUT_FORMAT"); v != "" {
		cfg.Output.Format = strings.ToLower(v)
	}
	if v := getenv("GITCONTRIBUTE_OUTPUT_MAX_RESULTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("GITCONTRIBUTE_OUTPUT_MAX_RESULTS: %w", err)
		}
		cfg.Output.MaxResults = n
	}
	return nil
}

// Validate checks that cfg is well-formed.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if cfg.Database == "" {
		return errors.New("database path must be set")
	}

	switch cfg.TokenSource.Method {
	case "none", "keyring", "gh-cli":
	case "env":
		if cfg.TokenSource.Key == "" {
			return errors.New("token_source key is required when method is env")
		}
	default:
		return fmt.Errorf("invalid token_source method %q", cfg.TokenSource.Method)
	}

	if cfg.Crawl.Budget <= 0 {
		return errors.New("crawl budget must be positive")
	}
	if cfg.Crawl.Concurrency <= 0 {
		return errors.New("crawl concurrency must be positive")
	}
	if cfg.Crawl.RetryLimit < 0 {
		return errors.New("crawl retry_limit must be non-negative")
	}
	if cfg.Crawl.Timeout != "" {
		if _, err := time.ParseDuration(cfg.Crawl.Timeout); err != nil {
			return fmt.Errorf("crawl timeout is not a valid duration: %w", err)
		}
	}

	switch cfg.Output.Format {
	case "text", "json":
	default:
		return fmt.Errorf("invalid output format %q", cfg.Output.Format)
	}
	if cfg.Output.MaxResults < 0 {
		return errors.New("output max_results must be non-negative")
	}

	return nil
}
