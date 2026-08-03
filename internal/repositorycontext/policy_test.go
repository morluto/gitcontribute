package repositorycontext

import "testing"

func TestGuidancePathsReturnsIndependentPolicyCopy(t *testing.T) {
	paths := GuidancePaths()
	if len(paths) == 0 || RequestCost() != len(paths)+2 {
		t.Fatalf("paths=%v request cost=%d", paths, RequestCost())
	}
	original := paths[0]
	paths[0] = "changed"
	if GuidancePaths()[0] != original {
		t.Fatal("guidance path policy leaked mutable storage")
	}
}
