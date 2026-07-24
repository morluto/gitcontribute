package cli

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type evidenceCmd struct {
	Add    addEvidenceCmd    `cmd:"" help:"Record evidence"`
	Show   showEvidenceCmd   `cmd:"" help:"Show evidence for an investigation"`
	Export exportEvidenceCmd `cmd:"" help:"Export an evidence packet"`
}

type addEvidenceCmd struct {
	Investigation string `name:"investigation" help:"Investigation ID"`
	Hypothesis    string `name:"hypothesis" help:"Hypothesis ID"`
	Opportunity   string `name:"opportunity" help:"Opportunity ID"`
	Type          string `name:"type" required:"" help:"Evidence type"`
	Relation      string `name:"relation" required:"" enum:"supporting,contradicting,inconclusive,stale,invalid" help:"Evidence relation"`
	Description   string `name:"description" required:"" help:"Evidence description"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type showEvidenceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) evidenceService() (contracts.EvidenceService, error) {
	service, ok := c.svc.(contracts.EvidenceService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runEvidence(ctx context.Context, command string, cmd *evidenceCmd) error {
	service, err := c.evidenceService()
	if err != nil {
		return err
	}
	switch command {
	case "evidence add":
		workflow, err := c.workflowService()
		if err != nil {
			return err
		}
		result, err := workflow.RecordEvidence(ctx, contracts.RecordEvidenceInput{
			InvestigationID: cmd.Add.Investigation, HypothesisID: cmd.Add.Hypothesis,
			OpportunityID: cmd.Add.Opportunity, Type: cmd.Add.Type,
			Relation: cmd.Add.Relation, Description: cmd.Add.Description,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.JSON, result)
	case "evidence show":
		result, err := service.ShowEvidence(ctx, cmd.Show.InvestigationID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	case "evidence export":
		return c.runExport(ctx, "export evidence", &exportCmd{Evidence: cmd.Export})
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown evidence command: %s", command))
	}
}
