package cli

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestArchiveHydrateHelpListsEveryLegalFacet(t *testing.T) {
	t.Parallel()
	help := commandHelp(t, "archive", "hydrate", "--help")
	for _, facet := range []string{
		"issue_comments", "issue_timeline", "pr_details", "pr_reviews", "pr_review_comments",
	} {
		if !strings.Contains(help, facet) {
			t.Errorf("archive hydrate help missing %q:\n%s", facet, help)
		}
	}
	if !strings.Contains(help, "defaults to") || !strings.Contains(help, "applicable non-timeline facets") {
		t.Errorf("archive hydrate help does not explain defaults:\n%s", help)
	}
}

func TestRadarHelpRequiresPositionalRepository(t *testing.T) {
	t.Parallel()
	help := commandHelp(t, "radar", "--help")
	if !strings.Contains(help, "Usage: gitcontribute radar <owner/repo>") {
		t.Fatalf("radar repository is not visibly required:\n%s", help)
	}
	if strings.Contains(help, "[<owner/repo>]") || strings.Contains(help, "--repo") {
		t.Fatalf("radar help exposes an optional or alternate repository argument:\n%s", help)
	}
}

func commandHelp(t *testing.T, args ...string) string {
	t.Helper()
	var root rootCmd
	var output strings.Builder
	parser, err := kong.New(
		&root,
		kong.Name("gitcontribute"),
		kong.Description("GitHub contribution research workbench"),
		kong.Writers(&output, &output),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(args); err != nil && output.Len() == 0 {
		t.Fatal(err)
	}
	return output.String()
}
