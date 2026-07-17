package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func testPaths(t *testing.T) *Paths {
	t.Helper()
	d := t.TempDir()
	return &Paths{
		OS:   "linux",
		Name: appName,
		Env: &Env{
			Home: d,
			Vars: map[string]string{
				"XDG_CONFIG_HOME": filepath.Join(d, ".config"),
				"XDG_DATA_HOME":   filepath.Join(d, ".local", "share"),
			},
		},
	}
}

func TestConfigRoundtrip(t *testing.T) {
	t.Parallel()
	paths := testPaths(t)
	cfg := &Config{
		TokenSource: TokenSource{Method: "env", Key: "GITHUB_TOKEN"},
		Crawl:       Crawl{Budget: 500, Concurrency: 8, RetryLimit: 2, Timeout: "1m"},
		Output:      Output{Format: "json", MaxResults: 50},
	}
	if err := ApplyDefaults(cfg, paths); err != nil {
		t.Fatalf("ApplyDefaults error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("roundtrip mismatch\ngot:  %+v\nwant: %+v", got, cfg)
	}
}

func TestConfigDefaultRoundtrip(t *testing.T) {
	t.Parallel()
	paths := testPaths(t)
	cfg := Default()
	if err := ApplyDefaults(cfg, paths); err != nil {
		t.Fatalf("ApplyDefaults error: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("default roundtrip mismatch\ngot:  %+v\nwant: %+v", got, cfg)
	}
}

func TestConfigSavePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions not applicable on Windows")
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := Default()
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("config file permissions = %o, want 0600", fi.Mode().Perm())
	}
}

func TestConfigUnknownFieldsRejected(t *testing.T) {
	t.Parallel()
	input := `
database = "/tmp/test.db"
unknown_key = "should fail"
`
	_, err := Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	var strictErr *toml.StrictMissingError
	if !errors.As(err, &strictErr) {
		t.Fatalf("expected *toml.StrictMissingError, got %T: %v", err, err)
	}
}

func TestConfigNestedUnknownFieldsRejected(t *testing.T) {
	t.Parallel()
	input := `
[crawl]
budget = 100
unknown_nested = 42
`
	_, err := Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unknown nested field, got nil")
	}
	var strictErr *toml.StrictMissingError
	if !errors.As(err, &strictErr) {
		t.Fatalf("expected *toml.StrictMissingError, got %T: %v", err, err)
	}
}

func TestConfigEnvOverrides(t *testing.T) {
	cfg := Default()
	cfg.Database = "/old/db"

	env := map[string]string{
		"GITCONTRIBUTE_DATABASE":            "/new/db",
		"GITCONTRIBUTE_TOKEN_SOURCE_METHOD": "env",
		"GITCONTRIBUTE_TOKEN_SOURCE_KEY":    "GH_TOKEN",
		"GITCONTRIBUTE_CRAWL_BUDGET":        "123",
		"GITCONTRIBUTE_CRAWL_CONCURRENCY":   "7",
		"GITCONTRIBUTE_CRAWL_RETRY_LIMIT":   "1",
		"GITCONTRIBUTE_CRAWL_TIMEOUT":       "5s",
		"GITCONTRIBUTE_OUTPUT_FORMAT":       "json",
		"GITCONTRIBUTE_OUTPUT_MAX_RESULTS":  "10",
	}
	getenv := func(k string) string { return env[k] }

	if err := ApplyEnv(cfg, getenv); err != nil {
		t.Fatalf("ApplyEnv error: %v", err)
	}

	if cfg.Database != "/new/db" {
		t.Fatalf("Database = %q, want %q", cfg.Database, "/new/db")
	}
	if cfg.TokenSource.Method != "env" || cfg.TokenSource.Key != "GH_TOKEN" {
		t.Fatalf("TokenSource = %+v, want env/GH_TOKEN", cfg.TokenSource)
	}
	if cfg.Crawl.Budget != 123 || cfg.Crawl.Concurrency != 7 || cfg.Crawl.RetryLimit != 1 || cfg.Crawl.Timeout != "5s" {
		t.Fatalf("Crawl = %+v", cfg.Crawl)
	}
	if cfg.Output.Format != "json" || cfg.Output.MaxResults != 10 {
		t.Fatalf("Output = %+v", cfg.Output)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name: "valid",
			cfg: &Config{
				Database:    "/tmp/db",
				TokenSource: TokenSource{Method: "gh-cli"},
				Crawl:       Crawl{Budget: 1, Concurrency: 1, RetryLimit: 0, Timeout: "1s"},
				Output:      Output{Format: "json", MaxResults: 0},
			},
		},
		{
			name:    "missing database",
			cfg:     &Config{Database: "", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "database path must be set",
		},
		{
			name:    "token env missing key",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "env"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "token_source key is required when method is env",
		},
		{
			name:    "token keyring missing account",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "keyring"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "token_source key is required when method is keyring",
		},
		{
			name:    "token keyring blank account",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "keyring", Key: "  \t"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "token_source key is required when method is keyring",
		},
		{
			name:    "invalid token method",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "magic"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "invalid token_source method",
		},
		{
			name:    "invalid budget",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 0, Concurrency: 1}, Output: Output{Format: "text"}},
			wantErr: "crawl budget must be positive",
		},
		{
			name:    "invalid concurrency",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: -1}, Output: Output{Format: "text"}},
			wantErr: "crawl concurrency must be positive",
		},
		{
			name:    "invalid retry limit",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: 1, RetryLimit: -1}, Output: Output{Format: "text"}},
			wantErr: "crawl retry_limit must be non-negative",
		},
		{
			name:    "invalid timeout",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: 1, Timeout: "not-a-duration"}, Output: Output{Format: "text"}},
			wantErr: "crawl timeout is not a valid duration",
		},
		{
			name:    "invalid output format",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "xml"}},
			wantErr: "invalid output format",
		},
		{
			name:    "negative max results",
			cfg:     &Config{Database: "/tmp/db", TokenSource: TokenSource{Method: "none"}, Crawl: Crawl{Budget: 1, Concurrency: 1}, Output: Output{Format: "text", MaxResults: -1}},
			wantErr: "output max_results must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	paths := testPaths(t)
	cfg := &Config{}
	if err := ApplyDefaults(cfg, paths); err != nil {
		t.Fatalf("ApplyDefaults error: %v", err)
	}

	want := Default()
	want.Database = cfg.Database // resolved from paths

	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("ApplyDefaults mismatch\ngot:  %+v\nwant: %+v", cfg, want)
	}

	if !strings.HasSuffix(cfg.Database, "gitcontribute.db") {
		t.Fatalf("unexpected default database path: %q", cfg.Database)
	}
}
