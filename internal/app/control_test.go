package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

type failingAuthSource struct{ err error }

func (s failingAuthSource) Token(context.Context) (string, error) { return "", s.err }

func TestMetadataIsLocalAndDoesNotCreateCorpus(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "v1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	result, err := svc.Metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "v1.2.3" || result.Name != "gitcontribute" {
		t.Fatalf("unexpected metadata: %+v", result)
	}
	if result.SchemaVersion != 0 {
		t.Fatalf("schema version = %d, want 0 before corpus open", result.SchemaVersion)
	}
	if !result.Features["contribution_radar"] || !containsString(result.Capabilities, "contribution-radar") {
		t.Fatalf("radar capability missing from metadata: %+v", result)
	}
	if !result.Features["contribution_readiness"] || !containsString(result.Capabilities, "contribution-readiness") {
		t.Fatalf("readiness capability missing from metadata: %+v", result)
	}
	if !result.Features["thread_research"] || !containsString(result.Capabilities, "thread-research-brief") {
		t.Fatalf("thread research capability missing from metadata: %+v", result)
	}
	if !result.Features["thread_investigation"] || !containsString(result.Capabilities, "thread-investigation-start") {
		t.Fatalf("thread investigation capability missing from metadata: %+v", result)
	}
	if !result.Features["evidence_freshness"] || !containsString(result.Capabilities, "evidence-freshness") {
		t.Fatalf("evidence freshness capability missing from metadata: %+v", result)
	}
	if _, err := os.Stat(result.CorpusPath); !os.IsNotExist(err) {
		t.Fatalf("metadata created corpus %q: %v", result.CorpusPath, err)
	}
}

func TestMetadataPropagatesOpenCorpusSchemaErrors(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() { svc.corpus = nil }()

	if _, err := svc.Metadata(context.Background()); err == nil || !strings.Contains(err.Error(), "read corpus schema version") {
		t.Fatalf("metadata schema error = %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestConfigureInvalidInputDoesNotReplaceFile(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	path, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	invalid := 0
	if _, err := svc.Configure(context.Background(), cli.ConfigureOptions{CrawlBudget: &invalid}); err == nil {
		t.Fatal("Configure succeeded with zero crawl budget")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid configuration changed the saved file")
	}
}

func TestConfigureDryRunDoesNotSave(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	budget := 25
	result, err := svc.Configure(context.Background(), cli.ConfigureOptions{CrawlBudget: &budget, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || !result.Changed || result.Config.CrawlBudget != budget {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("dry run created config: %v", err)
	}
}

func TestControlStatusUsesLocalCorpus(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordRateLimitObservation(context.Background(), corpus.RateLimitObservation{
		Attempt: 1, StatusCode: 200, Resource: "core", Limit: 5000, Remaining: 4999,
		ObservedAt: time.Unix(100, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ControlStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Healthy || result.SchemaVersion == 0 {
		t.Fatalf("unexpected status: %+v", result)
	}
	if result.Counts.Repositories != 0 || len(result.Warnings) == 0 {
		t.Fatalf("unexpected empty-corpus status: %+v", result)
	}
	if len(result.RateLimits) != 1 || result.RateLimits[0].Resource != "core" || result.RateLimits[0].Remaining != 4999 {
		t.Fatalf("unexpected rate-limit status: %+v", result.RateLimits)
	}
}

func TestDoctorDoesNotExposeEnvironmentToken(t *testing.T) {
	secret := strings.Join([]string{"github_pat", "fixture-control-value"}, "_")
	t.Setenv("GITCONTRIBUTE_TEST_TOKEN", secret)
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	method, key := "env", "GITCONTRIBUTE_TEST_TOKEN"
	if _, err := svc.Configure(context.Background(), cli.ConfigureOptions{TokenSource: &method, TokenSourceKey: &key}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) == "" || strings.Contains(string(payload), secret) {
		t.Fatalf("doctor output exposed a token: %s", payload)
	}
}

func TestDoctorInspectsEffectiveRuntimeConfig(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	envDB := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("GITCONTRIBUTE_DATABASE", envDB)

	result, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !result.Healthy {
		t.Fatalf("doctor reported unhealthy: %+v", result)
	}
	if svc.databasePath() != envDB {
		t.Fatalf("doctor did not apply env override: got %q, want %q", svc.databasePath(), envDB)
	}
}

func TestConfigureDoesNotPersistEnvOverrides(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	defaultDB, err := paths.DatabasePath()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	envDB := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("GITCONTRIBUTE_DATABASE", envDB)

	budget := 25
	result, err := svc.Configure(context.Background(), cli.ConfigureOptions{CrawlBudget: &budget})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if result.Config.Database == envDB {
		t.Fatalf("configure persisted env override: %q", result.Config.Database)
	}
	if result.Config.Database != defaultDB {
		t.Fatalf("configure changed database to %q, want %q", result.Config.Database, defaultDB)
	}

	path, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), envDB) {
		t.Fatalf("config file contains env-override database path")
	}
}

func TestCheckAuthSourceProbesConfiguredKeyring(t *testing.T) {
	want := errors.New("keyring backend unavailable")
	cfg := config.Default()
	cfg.TokenSource.Method = "keyring"
	cfg.TokenSource.Key = "account"

	err := checkAuthSource(context.Background(), cfg, failingAuthSource{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("checkAuthSource error = %v, want %v", err, want)
	}

	if err := checkAuthSource(context.Background(), cfg, github.StaticTokenSource("present")); err != nil {
		t.Fatalf("available keyring source rejected: %v", err)
	}
}
