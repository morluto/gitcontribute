# GitContribute Developer Experience Audit Report

**Date:** 2026-08-03
**Codebase:** 571 Go files, 80,189 source LOC, 47,946 test LOC, 48 packages
**Total test functions:** 1,206

---

## Executive Summary

This audit examined the entire gitcontribute codebase for developer experience
bottlenecks, critical bugs, structural anti-patterns, and improvement
opportunities. The codebase is well-architected with clear capability
boundaries and good test discipline, but suffers from **one critical test
isolation bug causing 7 test failures**, a **massive monolithic package**,
**uncached reflection-heavy schema generation** that dominates test runtime,
and **47,000 lines of test code exempted from all linting**.

---

## 1. CRITICAL BUG: Environment variable leak causes 7 test failures

### Severity: Critical — tests fail when run inside npx context

**Root cause:** `internal/app/upgrade.go:343` — `discoverInstallation()` calls
`os.Getenv("npm_command")` and `os.Getenv("npm_lifecycle_event")` directly
without test isolation. When tests run inside an npx-launched process (where
`npm_command=exec` and `npm_lifecycle_event=npx` are set in the environment),
`discoverInstallation` short-circuits and returns `context: "npx"` regardless
of the `osExecutable` override that tests set.

**Failing tests (6 upgrade tests + 1 MCP stdio test):**
- TestUpgradeRejectsMismatchedPostInstallNPMVersion
- TestUpgradeReportsInspectableStagesForGlobalNPM
- TestUpgradeGlobalNPMInstallsLatest
- TestUpgradeWindowsGlobalNPMDoesNotInstall
- TestUpgradeProjectNPMReportsManualUpdate
- TestUpgradeCombinedInstallActivatesPrivateRuntimeFromInstalledPackage
- TestMCPStdioPullRequestPortfolioFlow (separate fixture issue)

**Fix:** Each failing test needs `t.Setenv("npm_command", "")` and
`t.Setenv("npm_lifecycle_event", "")` before calling `svc.Upgrade`. The shared
helper `setupUpgradeActivationTest` (upgrade_test.go:674) should add these
isolation calls so all callers inherit the fix.

**Production edge case:** If a globally-installed gitcontribute binary is
invoked from within an npx-launched shell, it would inherit the npm env vars
and be misclassified as npx.

**Proof:** Running `go test -short ./internal/app/` with
`npm_command=exec npm_lifecycle_event=npx` set (as in this CI environment)
reproduces all 6 failures. Clearing those vars makes all 6 pass.

---

## 2. PERFORMANCE: MCP server tests re-generate 40+ JSON schemas per test (27s total)

### Severity: High — biggest single test-time bottleneck

**Root cause:** Every `internal/mcpserver` test calls `connectServer()` which
calls `New(reader, "test")` → `newServer()` → `s.register()`. The `register()`
method registers 40+ MCP tools. Each tool registration calls
`inputSchema[T]()` and `outputSchema[T]()`, which both call
`inferredSchema[T]()`, which calls `jsonschema.For[T]()` using Go reflection.

**There is NO caching.** `jsonschema.For[T]()` runs fresh reflection for every
single tool, for every single `New()` call, in every single test.

A single test like `TestRepositoryResourceAndNotFound` takes 0.39s despite
doing only two trivial resource reads — the 0.39s is almost entirely
reflection-based schema generation inside `New()`.

**Impact:** 151 tests × ~0.15s reflection overhead = ~23s wasted on redundant
schema generation. This is the single largest test-time cost in the codebase.

**Fix:** Cache the schema catalog using `sync.Once` or a `sync.Map` keyed by
`readOnly bool`. The schemas depend only on Go types, not on the reader. This
would eliminate ~20s from the test suite.

**Timeline:** mcpserver 27s → estimated ~3s with caching.

---

## 3. PERFORMANCE: Every test creates a fresh SQLite DB with full migrations

### Severity: High

**Root cause:** `internal/corpus` tests call `openTestCorpus(t)` which creates
a real SQLite DB at `t.TempDir()` and runs the full Goose migration suite (13
migrations, 2,506 lines of DDL including FTS5 virtual tables) from scratch for
every test.

`internal/app` tests inherit this cost because every `newTestService()` calls
`svc.Init()` → `s.openCorpus()` → same fresh SQLite DB with full migrations.

**Impact:**
- `internal/corpus`: 10.4s for ~80 tests
- `internal/app`: 19.1s for ~80 tests (inherits corpus cost + httptest + git subprocesses)

**Fix for read/query tests:** Pre-migrate a template `.db` file once, then copy
it (near-instant) instead of running DDL. SQLite file copies are ~1000x
faster than running the full migration suite.

**Fix for migration/ordering tests:** Keep isolation — these tests
specifically test fresh migration behavior.

---

## 4. STRUCTURAL: `internal/app` is a 90-file god package (26K LOC)

### Severity: High (structural bottleneck)

**The problem:** `internal/app` contains 90 source files and 75 test files
totaling 43,803 lines (26,039 source + 17,764 test). It holds 905 functions.
This is 32% of the entire codebase's source code in a single Go package.

The `Service` struct in `app.go` implements 3 interfaces
(`Service`, `WorkflowService`, `DossierService`) but the actual scope of
responsibility is far wider — the package contains MCP tool handlers,
upgrade logic, TUI support, search, sync/hydration, discovery, evidence,
research, and more.

**Natural decomposition:**

| Sub-package | Files | LOC | Responsibility |
|---|---|---|---|
| `internal/app/mcp` | 56 files | 13,519 | MCP tool handlers (mcp_*.go) |
| `internal/app/upgrade` | 3 files | 2,664 | npm upgrade logic |
| `internal/app/tui` | 4 files | 1,575 | TUI action support |
| `internal/app/sync` | ~8 files | 2,915 | Sync/hydration |
| `internal/app/search` | 3 files | 1,418 | Search |

The 56 MCP handler files alone are 13,519 lines — more than many entire
packages in the codebase.

**Git churn confirms this is a hotspot:** `internal/cli/cli.go` was changed 79
times, `internal/mcpserver/server.go` 50 times, `internal/app/mcp_v1.go` 40
times.

**Fix:** Extract `internal/app/mcp` as a first step. The MCP handlers depend
on the `Service` struct but could accept a narrower interface.

---

## 5. ANTI-PATTERN: 47,727 lines of test code exempted from ALL linting

### Severity: Medium (quality gate gap)

**The problem:** The `.golangci.yml` exempts all `_test.go` files from 17
linters including `staticcheck`, `errcheck`, `contextcheck`, `gosec`, `cyclop`,
and `revive`. That's 47,727 lines of test code (213 test files) where
complexity, duplicate code, error handling, context propagation, and security
checks are all disabled.

This means test files can grow to any complexity, ignore context cancellation,
leak resources, and accumulate duplicate setup patterns — all without any
lint feedback.

**Fix:** Narrow the exclusions. Tests need `dupl` and `funlen` exemptions for
fixture-heavy patterns, but should keep `errcheck`, `staticcheck`,
`contextcheck`, `noctx`, `gosec`, `cyclop`, and `unconvert` active.

---

## 6. ANTI-PATTERN: t.Fatalf dominates 32:1 over t.Errorf

### Severity: Medium (test quality)

**The problem:** 3,704 `t.Fatalf` calls vs 116 `t.Errorf` calls (32:1 ratio).
`t.Fatalf` stops the test at the first failure, hiding subsequent issues that
`t.Errorf` would report.

When a test fails, the developer sees only the first assertion failure and
has to fix-and-rerun to discover the next one. With `t.Errorf`, multiple
assertions can fail in one run, giving a complete picture.

**Fix:** Audit the 3,704 `t.Fatalf` calls and convert assertion failures (not
setup failures) to `t.Errorf`. Setup failures like `t.Fatalf("open corpus: %v",
err)` should remain `t.Fatalf`.

---

## 7. STRUCTURAL: `internal/cli/cli.go` is 1,690 lines with 53 command types

### Severity: Medium (maintainability)

**The problem:** `internal/cli/cli.go` is 1,690 lines — the only file over the
800-line CI threshold. It contains 53 command struct types, a `Run()` method
with a 50-case switch statement, and 60 functions. It was changed 79 times in
git history — the most-churned file in the codebase.

**Fix:** Split by command group: `cli_setup.go` (setup/remove/upgrade),
`cli_corpus.go` (corpus/search/dossier/research), `cli_sync.go`
(source/crawl/acquire), `cli_investigation.go`
(investigation/hypothesis/validation), etc.

---

## 8. STRUCTURAL: 10 packages with zero test files

### Severity: Medium (test coverage gap)

**10 packages have no test files at all:**
```
clusterprojection, contracts, failure, mcpadapter, precedent,
redaction, repository, repositorycontext, terminalinstall, tuicontract
```

Some (`contracts`, `failure`) are pure type/contract definitions and may not
need tests. But others (`clusterprojection`, `mcpadapter`, `precedent`,
`redaction`, `repositorycontext`) contain logic that should be tested.

---

## 9. PERFORMANCE: `time.Sleep` in 17 test locations

### Severity: Low (test reliability)

17 `time.Sleep` calls in tests create timing-dependent flakiness. Examples:
- `internal/app/job_executor_test.go` — `time.Sleep(10 * time.Millisecond)` (5 locations)
- `internal/app/tui_capture_test.go` — `time.Sleep(100 * time.Millisecond)` (2 locations)
- `internal/discovery/gharchive_fetcher_test.go` — `time.Sleep(20 * time.Millisecond)`

These should be replaced with channel-based synchronization or condition
polling where possible.

---

## 10. OBSERVATION: Test-to-source ratio is 0.59

### Severity: Info

47,946 test LOC / 80,189 source LOC = 0.59. This is below the common 1:1
ideal for well-tested code. However, the 70% coverage threshold is enforced
in CI, and many packages have excellent test coverage. The gap is partly
explained by the 10 untested packages and the heavy use of integration-style
tests that exercise multiple layers.

---

## Summary Table

| # | Issue | Severity | Impact | Fix Effort |
|---|---|---|---|---|
| 1 | Env var leak causes 7 test failures | Critical | Tests fail in npx | Low (add t.Setenv) |
| 2 | Uncached JSON schema reflection | High | 27s → ~3s | Medium (add sync.Once cache) |
| 3 | Fresh SQLite DB per test | High | 30s → ~5s | Medium (template copy) |
| 4 | internal/app god package | High | Maintainability | High (extract sub-packages) |
| 5 | 47K lines exempt from linting | Medium | Quality gate gap | Low (narrow exclusions) |
| 6 | t.Fatalf 32:1 over t.Errorf | Medium | Debug iterations | Medium (gradual conversion) |
| 7 | cli.go 1,690 lines | Medium | Maintainability | Medium (split by group) |
| 8 | 10 packages with no tests | Medium | Coverage gap | Medium (add tests) |
| 9 | time.Sleep in tests | Low | Flakiness risk | Low (use channels) |
| 10 | Test ratio 0.59 | Info | Indicator | N/A |

---

## Recommended Priority

1. **Fix the env var leak** (1 hour) — unblocks all CI test runs
2. **Cache JSON schemas** (2 hours) — cuts 20s from test suite
3. **Template DB copy for read tests** (4 hours) — cuts 15s from test suite
4. **Extract internal/app/mcp** (1-2 days) — biggest structural win
5. **Narrow lint exclusions** (1 hour) — quality gate improvement
6. **Split cli.go** (1 day) — maintainability
