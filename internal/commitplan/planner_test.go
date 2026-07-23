package commitplan

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const mixedPatch = `diff --git a/service.go b/service.go
index 1111111111111111111111111111111111111111..2222222222222222222222222222222222222222 100644
--- a/service.go
+++ b/service.go
@@ -1,2 +1,2 @@
-old startup
+new startup
 context
@@ -10,2 +10,2 @@
-old search
+new search
 context
diff --git a/service_test.go b/service_test.go
index 3333333333333333333333333333333333333333..4444444444444444444444444444444444444444 100644
--- a/service_test.go
+++ b/service_test.go
@@ -1,2 +1,2 @@
-old test
+new test
 context
`

func TestBuildProvesExactCoverageAcrossMixedFile(t *testing.T) {
	t.Parallel()
	inventory, err := Inspect(context.Background(), Snapshot{Patch: []byte(mixedPatch)})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Units) != 3 {
		t.Fatalf("units = %+v", inventory.Units)
	}
	plan, err := Build(context.Background(), inventory, PlanInput{Groups: []GroupInput{
		{Name: "startup", Intent: "coordinate startup", Type: "fix", Scope: "runtime", UnitIDs: []string{inventory.Units[0].ID, inventory.Units[2].ID}, ValidationCommands: []string{"go test ./internal/runtime"}},
		{Name: "search", Intent: "bound local search", Type: "perf", UnitIDs: []string{inventory.Units[1].ID}, DependsOn: []string{"startup"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Reconstruction.Verified || plan.Reconstruction.AssignedSHA256 != inventory.InventorySHA256 {
		t.Fatalf("reconstruction = %+v", plan.Reconstruction)
	}
	if plan.Groups[0].SuggestedSubject != "fix(runtime): coordinate startup" {
		t.Fatalf("subject = %q", plan.Groups[0].SuggestedSubject)
	}
	foundMixed := false
	for _, warning := range plan.Warnings {
		foundMixed = foundMixed || warning.Code == "mixed_file" && warning.Path == "service.go"
	}
	if !foundMixed {
		t.Fatalf("warnings = %+v", plan.Warnings)
	}
}

func TestInspectPreservesRenameGeneratedAndUntrackedSemantics(t *testing.T) {
	t.Parallel()
	patch := `diff --git a/old.snap b/new.snap
similarity index 100%
rename from old.snap
rename to new.snap
`
	inventory, err := Inspect(context.Background(), Snapshot{Patch: []byte(patch), Untracked: []UntrackedFile{{Path: "fixtures/new.bin", ObjectID: "blob-123"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Units) != 2 || inventory.Units[0].Operation != "rename" || !inventory.Units[0].Generated || inventory.Units[1].Kind != "untracked" {
		t.Fatalf("inventory = %+v", inventory)
	}
	if inventory.SourcePatchSHA256 == "" || inventory.InventorySHA256 == "" {
		t.Fatalf("missing proof digests: %+v", inventory)
	}
	plan, err := Build(context.Background(), inventory, PlanInput{Groups: []GroupInput{
		{Name: "snapshot", Intent: "rename snapshot", Type: "test", UnitIDs: []string{inventory.Units[0].ID}},
		{Name: "fixture", Intent: "add binary fixture", Type: "test", UnitIDs: []string{inventory.Units[1].ID}},
	}})
	if err != nil || !plan.Reconstruction.Verified || len(plan.Groups[0].Files) != 1 || len(plan.Groups[1].Files) != 1 {
		t.Fatalf("pure-file plan = %+v, %v", plan, err)
	}
}

func TestInspectTreatsBinaryPatchAsIndivisible(t *testing.T) {
	t.Parallel()
	patch := `diff --git a/image.png b/image.png
index 1111111111111111111111111111111111111111..2222222222222222222222222222222222222222 100644
GIT binary patch
literal 1
HcmV?d00001

`
	inventory, err := Inspect(context.Background(), Snapshot{Patch: []byte(patch)})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Units) != 1 || inventory.Units[0].Operation != "binary" || inventory.Units[0].Kind != "file" {
		t.Fatalf("binary inventory = %+v", inventory)
	}
	if len(inventory.Warnings) != 1 || inventory.Warnings[0].Code != "binary_file" {
		t.Fatalf("binary warnings = %+v", inventory.Warnings)
	}
}

func TestBuildLeavesAmbiguousHunkUnresolved(t *testing.T) {
	t.Parallel()
	inventory, err := Inspect(context.Background(), Snapshot{Patch: []byte(mixedPatch)})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(context.Background(), inventory, PlanInput{
		Groups:     []GroupInput{{Name: "known", Intent: "update startup", Type: "fix", UnitIDs: []string{inventory.Units[0].ID}}},
		Unresolved: []UnresolvedInput{{UnitID: inventory.Units[1].ID, Reason: "could belong to startup or search"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Reconstruction.Verified || len(plan.Unresolved) != 2 || plan.Unresolved[0].Reason != "could belong to startup or search" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildRejectsDuplicateAssignmentAndDependencyCycle(t *testing.T) {
	t.Parallel()
	inventory, err := Inspect(context.Background(), Snapshot{Patch: []byte(mixedPatch)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Build(context.Background(), inventory, PlanInput{Groups: []GroupInput{
		{Name: "one", Intent: "one", Type: "fix", UnitIDs: []string{inventory.Units[0].ID}},
		{Name: "two", Intent: "two", Type: "test", UnitIDs: []string{inventory.Units[0].ID}},
	}})
	if err == nil || !strings.Contains(err.Error(), "assigned to both") {
		t.Fatalf("duplicate assignment error = %v", err)
	}
	_, err = Build(context.Background(), inventory, PlanInput{Groups: []GroupInput{
		{Name: "one", Intent: "one", Type: "fix", UnitIDs: []string{inventory.Units[0].ID}, DependsOn: []string{"two"}},
		{Name: "two", Intent: "two", Type: "test", UnitIDs: []string{inventory.Units[1].ID}, DependsOn: []string{"one"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("dependency cycle error = %v", err)
	}
}

func TestPlannerHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, Snapshot{Patch: []byte(mixedPatch)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("inspect cancellation = %v", err)
	}
	if _, err := Build(ctx, Inventory{}, PlanInput{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("build cancellation = %v", err)
	}
}
