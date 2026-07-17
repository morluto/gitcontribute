package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

func TestMetadataIsLocalAndDoesNotCreateCorpus(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "v1.2.3")
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
	if _, err := os.Stat(result.CorpusPath); !os.IsNotExist(err) {
		t.Fatalf("metadata created corpus %q: %v", result.CorpusPath, err)
	}
}

func TestConfigureInvalidInputDoesNotReplaceFile(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
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
	svc, err := New(paths, "test")
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
	svc, err := New(paths, "test")
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
	const secret = "github_pat_secret-control-test"
	t.Setenv("GITCONTRIBUTE_TEST_TOKEN", secret)
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
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
