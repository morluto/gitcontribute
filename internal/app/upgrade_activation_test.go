package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
	_ "modernc.org/sqlite"
)

func TestUpgradeBlocksActivationWhenTargetSchemaExceedsCorpus(t *testing.T) {
	home, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	dbPath := filepath.Join(home, "gitcontribute.db")
	svc.cfg.Database = dbPath
	setRuntimeContract(t, "1.2.4", 999)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO goose_db_version (id, version_id) VALUES (1, 1)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	assertStage(t, report, "activation", "failed")
	assertStage(t, report, "corpus-schema", "migration_required")
	if report.Status != "schema migration required" {
		t.Fatalf("status = %q, want schema migration required", report.Status)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed when target schema exceeds corpus")
	}
}

func TestUpgradeBlocksActivationWithInvalidRuntimeContract(t *testing.T) {
	_, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContractOutput(t, "not-json")

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	assertStage(t, report, "activation", "failed")
	if report.Status != "activation failed" {
		t.Fatalf("status = %q, want activation failed", report.Status)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed with invalid runtime contract")
	}
}

func TestUpgradeReportsTargetRuntimeUnavailableWhenNoStagedExecutable(t *testing.T) {
	_, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "")

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	assertStage(t, report, "activation", "target_runtime_unavailable")
	if report.Status != "target runtime unavailable" {
		t.Fatalf("status = %q, want target runtime unavailable", report.Status)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed without a staged target executable")
	}
}

func TestUpgradeBlocksActivationWithTrailingRuntimeContract(t *testing.T) {
	_, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContractOutput(t, `{"name":"gitcontribute","version":"1.2.4","supported_schema_version":1}{"unexpected":"second value"}`)

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	assertStage(t, report, "activation", "failed")
	if report.Status != "activation failed" {
		t.Fatalf("status = %q, want activation failed", report.Status)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed with trailing runtime contract data")
	}
}

func TestReadRuntimeContractAcceptsUnknownFields(t *testing.T) {
	original := runtimeContractCommand
	t.Cleanup(func() { runtimeContractCommand = original })
	var gotPath string
	runtimeContractCommand = func(_ context.Context, path string) ([]byte, error) {
		gotPath = path
		return []byte(`{"name":"gitcontribute","version":"1.2.4","supported_schema_version":28,"future_field":{"enabled":true}}`), nil
	}
	contract, err := readRuntimeContract(context.Background(), "/release/candidate")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/release/candidate" || contract.Version != "1.2.4" || contract.SupportedSchemaVersion != 28 {
		t.Fatalf("path=%q contract=%+v", gotPath, contract)
	}
}

func TestUpgradeRejectsDestinationRuntimeContractDisagreementBeforeRegistration(t *testing.T) {
	_, source, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	dataDir, err := svc.paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	destination, err := managedbinary.Destination(dataDir, "1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	original := runtimeContractCommand
	t.Cleanup(func() { runtimeContractCommand = original })
	var paths []string
	runtimeContractCommand = func(_ context.Context, path string) ([]byte, error) {
		paths = append(paths, path)
		schema := 1
		if filepath.Clean(path) == filepath.Clean(destination) {
			schema = 2
		}
		return []byte(fmt.Sprintf(`{"name":"gitcontribute","version":"1.2.4","supported_schema_version":%d}`, schema)), nil
	}

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertStage(t, report, "activation", "failed")
	if !reflect.DeepEqual(paths, []string{source, destination}) {
		t.Fatalf("runtime contract paths = %v, want source then destination", paths)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed when installed runtime contract disagreed")
	}
}

func TestUpgradeRejectsMismatchedPostInstallNPMVersion(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	originalGOOS := upgradeGOOS
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
		upgradeGOOS = originalGOOS
	})
	upgradeGOOS = "linux"

	home := t.TempDir()
	globalRoot := filepath.Join(home, "global", "lib", "node_modules")
	pkgRoot := filepath.Join(globalRoot, "gitcontribute")
	exe := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")

	osExecutable = func() (string, error) { return exe, nil }
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "npm" && len(args) >= 2 && args[0] == "root" && args[1] == "--global":
			return []byte(globalRoot + "\n"), nil
		case name == "npm" && len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute":
			return []byte("1.2.4\n"), nil
		case name == "npm" && len(args) >= 2 && args[0] == "install" && args[1] == "--global":
			// package.json is intentionally left at 1.2.3
			return nil, nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}

	svc := testService(t, home, "1.2.3", "")
	_, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err == nil {
		t.Fatal("expected error when installed npm version does not match target")
	}
	if !strings.Contains(err.Error(), "verify installed npm release") {
		t.Fatalf("error = %v", err)
	}
}

func TestUpgradeBlocksActivationWhenRuntimeContractLacksSupportedSchema(t *testing.T) {
	_, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContractOutput(t, `{"name":"gitcontribute","version":"1.2.4"}`)

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	assertStage(t, report, "activation", "failed")
	if report.Status != "activation failed" {
		t.Fatalf("status = %q, want activation failed", report.Status)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed with missing supported schema")
	}
}

func TestPrivateActivationReportsIncompleteRollback(t *testing.T) {
	report := &cli.UpgradeReport{}
	svc := &Service{}
	svc.setPrivateActivationFailure(report, 2, &clientsetup.ActivationRollbackError{
		Cause:    errors.New("verification failed"),
		Rollback: errors.New("registration changed concurrently"),
	})
	assertStage(t, report, "activation", "rollback_failed")
	if report.Status != "activation rollback failed" || !strings.Contains(report.Action, "inspect") {
		t.Fatalf("rollback failure report = %+v", report)
	}
}
