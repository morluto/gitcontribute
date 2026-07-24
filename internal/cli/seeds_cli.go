package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/domain"
)

func (c *CLI) runSeeds(ctx context.Context, cmd *seedsCmd) error {
	if cmd.Limit <= 0 || cmd.Limit > 100 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 100"))
	}
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	service, err := c.dossierService()
	if err != nil {
		return err
	}
	opts, err := parseSeedOptions(splitCSV(cmd.From), splitCSV(cmd.Polarity), cmd.Limit)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	result, err := service.ExtractSeeds(ctx, repo, opts)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func parseSeedOptions(classes, polarities []string, limit int) (domain.ExtractSeedsOptions, error) {
	parsedClasses := make([]domain.SeedSourceClass, len(classes))
	for i, class := range classes {
		switch class {
		case "merged-prs", "merged_pr", "merged_prs":
			parsedClasses[i] = domain.SeedSourceClassMergedPR
		case "closed-prs", "closed_unmerged_pr", "closed_unmerged_prs":
			parsedClasses[i] = domain.SeedSourceClassClosedUnmergedPR
		case "issues", "issue":
			parsedClasses[i] = domain.SeedSourceClassIssue
		default:
			return domain.ExtractSeedsOptions{}, fmt.Errorf("unknown seed source class %q", class)
		}
	}
	parsedPolarities := make([]domain.SeedPolarity, 0, len(polarities))
	for _, polarity := range polarities {
		switch polarity {
		case "positive":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityPositive)
		case "negative":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityNegative)
		case "context":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityContext)
		case "all":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityPositive, domain.SeedPolarityNegative, domain.SeedPolarityContext)
		default:
			return domain.ExtractSeedsOptions{}, fmt.Errorf("unknown seed polarity %q", polarity)
		}
	}
	return domain.ExtractSeedsOptions{Classes: parsedClasses, Polarities: parsedPolarities, Limit: limit}, nil
}
