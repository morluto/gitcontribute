package clusterprojection

import (
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestIdentityMatchesAllProjectionInputs(t *testing.T) {
	identity := Identity{SourceRevision: "source-1", GovernanceRevision: 3, RuleVersion: similarity.RuleVersion("duplicate-v1")}
	if !identity.Matches("source-1", 3, similarity.RuleVersion("duplicate-v1")) {
		t.Fatal("identity did not match equal inputs")
	}
	for name, args := range map[string]struct {
		source     string
		governance uint64
		rule       similarity.RuleVersion
	}{
		"source":     {source: "source-2", governance: 3, rule: identity.RuleVersion},
		"governance": {source: identity.SourceRevision, governance: 4, rule: identity.RuleVersion},
		"rule":       {source: identity.SourceRevision, governance: 3, rule: similarity.RuleVersion("duplicate-v2")},
	} {
		if identity.Matches(args.source, args.governance, args.rule) {
			t.Errorf("identity matched changed %s input", name)
		}
	}
}

func TestStaleInputErrorDescribesBothRevisionChanges(t *testing.T) {
	err := (&StaleInputError{ExpectedSource: "old", ActualSource: "new", ExpectedGovernance: 1, ActualGovernance: 2}).Error()
	if !strings.Contains(err, `source "old" -> "new"`) || !strings.Contains(err, "governance 1 -> 2") {
		t.Fatalf("stale error = %q", err)
	}
}
