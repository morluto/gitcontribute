package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	maxTrackingListLimit       = 1000
	maxMetadataImportBytes     = 64 << 20
	maxMetadataImportBytesHint = "64MB"
)

type triageCmd struct {
	Record triageRecordCmd `cmd:"" help:"Record a triage outcome"`
	List   triageListCmd   `cmd:"" help:"List triage outcomes"`
}

type triageRecordCmd struct {
	Target  string `arg:"" help:"Target as kind:ref (e.g. repo:owner/repo, issue:owner/repo#12, opportunity:ID)"`
	Outcome string `arg:"" enum:"viewed,ignored,saved,investigated,implemented,submitted,merged,rejected,abandoned" help:"Triage outcome"`
	Reason  string `name:"reason" help:"Optional reason"`
	Lens    string `name:"lens" help:"Optional lens name"`
	JSON    bool   `name:"json" help:"Print the result as JSON"`
}

type triageListCmd struct {
	Kind    string `name:"kind" help:"Filter by target kind"`
	Ref     string `name:"ref" help:"Filter by target ref"`
	Outcome string `name:"outcome" help:"Filter by outcome"`
	Lens    string `name:"lens" help:"Filter by lens"`
	Limit   int    `name:"limit" default:"20" help:"Maximum events to return"`
	JSON    bool   `name:"json" help:"Print the result as JSON"`
}

type contributionCmd struct {
	Record   contributionRecordCmd   `cmd:"" help:"Record a contribution"`
	List     contributionListCmd     `cmd:"" help:"List contributions"`
	Show     contributionShowCmd     `cmd:"" help:"Show a contribution"`
	Outcome  contributionOutcomeCmd  `cmd:"" help:"Record a contribution outcome"`
	Outcomes contributionOutcomesCmd `cmd:"" name:"outcomes" help:"List contribution outcomes"`
}

type contributionRecordCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	Kind          string `arg:"" enum:"issue,pull_request,pr" help:"Contribution kind"`
	Title         string `arg:"" help:"Contribution title"`
	Body          string `name:"body" help:"Body text"`
	Reference     string `name:"reference" help:"External reference"`
	ReferenceURL  string `name:"reference-url" help:"External reference URL"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type contributionListCmd struct {
	OpportunityID string `name:"opportunity" help:"Filter by opportunity ID"`
	Kind          string `name:"kind" help:"Filter by kind"`
	Limit         int    `name:"limit" default:"20" help:"Maximum contributions to return"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type contributionShowCmd struct {
	ID   string `arg:"" help:"Contribution ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type contributionOutcomeCmd struct {
	ContributionID string `arg:"" help:"Contribution ID"`
	Outcome        string `arg:"" enum:"submitted,merged,rejected,abandoned" help:"Contribution outcome"`
	Reason         string `name:"reason" help:"Optional reason"`
	JSON           bool   `name:"json" help:"Print the result as JSON"`
}

type contributionOutcomesCmd struct {
	ContributionID string `arg:"" help:"Contribution ID"`
	JSON           bool   `name:"json" help:"Print the result as JSON"`
}

type trackingCmd struct {
	Export trackingExportCmd `cmd:"" help:"Export local tracking metadata"`
	Import trackingImportCmd `cmd:"" help:"Import local tracking metadata"`
}

type trackingExportCmd struct {
	Output string `name:"output" help:"Write to a file instead of stdout"`
	Limit  int    `name:"limit" default:"10000" help:"Maximum records to export"`
	JSON   bool   `name:"json" help:"Print a JSON summary instead of the raw bundle"`
}

type trackingImportCmd struct {
	File string `name:"file" help:"Read from a file ('-' for stdin)"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) trackingService() (TrackingService, error) {
	service, ok := c.svc.(TrackingService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runTriage(ctx context.Context, command string, cmd *triageCmd) error {
	service, err := c.trackingService()
	if err != nil {
		return err
	}
	switch command {
	case "triage record":
		fmt.Fprintf(c.stderr, "recording triage outcome for %s...\n", cmd.Record.Target)
		result, err := service.RecordTriageEvent(ctx, RecordTriageEventOptions{
			Target:  cmd.Record.Target,
			Outcome: cmd.Record.Outcome,
			Reason:  cmd.Record.Reason,
			Lens:    cmd.Record.Lens,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Record.JSON, result)
	case "triage list":
		if cmd.List.Limit <= 0 || cmd.List.Limit > maxTrackingListLimit {
			return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and %d", maxTrackingListLimit))
		}
		result, err := service.ListTriageEvents(ctx, ListTriageEventsOptions{
			TargetKind: cmd.List.Kind,
			TargetRef:  cmd.List.Ref,
			Outcome:    cmd.List.Outcome,
			Lens:       cmd.List.Lens,
			Limit:      cmd.List.Limit,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown triage command: %s", command))
	}
}

func (c *CLI) runContribution(ctx context.Context, command string, cmd *contributionCmd) error {
	service, err := c.trackingService()
	if err != nil {
		return err
	}
	switch command {
	case "contribution record":
		fmt.Fprintf(c.stderr, "recording contribution for opportunity %s...\n", cmd.Record.OpportunityID)
		result, err := service.RecordContribution(ctx, RecordContributionOptions{
			OpportunityID: cmd.Record.OpportunityID,
			Kind:          cmd.Record.Kind,
			Title:         cmd.Record.Title,
			Body:          cmd.Record.Body,
			Reference:     cmd.Record.Reference,
			ReferenceURL:  cmd.Record.ReferenceURL,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Record.JSON, result)
	case "contribution list":
		if cmd.List.Limit <= 0 || cmd.List.Limit > maxTrackingListLimit {
			return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and %d", maxTrackingListLimit))
		}
		result, err := service.ListContributions(ctx, ListContributionsOptions{
			OpportunityID: cmd.List.OpportunityID,
			Kind:          cmd.List.Kind,
			Limit:         cmd.List.Limit,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	case "contribution show":
		result, err := service.GetContribution(ctx, cmd.Show.ID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	case "contribution outcome":
		fmt.Fprintf(c.stderr, "recording outcome %s for contribution %s...\n", cmd.Outcome.Outcome, cmd.Outcome.ContributionID)
		result, err := service.RecordContributionOutcome(ctx, RecordContributionOutcomeOptions{
			ContributionID: cmd.Outcome.ContributionID,
			Outcome:        cmd.Outcome.Outcome,
			Reason:         cmd.Outcome.Reason,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Outcome.JSON, result)
	case "contribution outcomes":
		result, err := service.ListContributionOutcomes(ctx, cmd.Outcomes.ContributionID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Outcomes.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown contribution command: %s", command))
	}
}

func (c *CLI) runTracking(ctx context.Context, command string, cmd *trackingCmd) error {
	service, err := c.trackingService()
	if err != nil {
		return err
	}
	switch command {
	case "tracking export":
		return c.runTrackingExport(ctx, &cmd.Export, service)
	case "tracking import":
		return c.runTrackingImport(ctx, &cmd.Import, service)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown tracking command: %s", command))
	}
}

func (c *CLI) runTrackingExport(ctx context.Context, cmd *trackingExportCmd, service TrackingService) error {
	if cmd.Limit <= 0 || cmd.Limit > 100000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 100000"))
	}
	fmt.Fprintln(c.stderr, "exporting local tracking metadata...")
	result, err := service.ExportLocalMetadata(ctx, MetadataExportOptions{Limit: cmd.Limit})
	if err != nil {
		return c.mapError(err)
	}
	if cmd.JSON {
		return c.render(true, result)
	}
	if cmd.Output != "" {
		if err := os.WriteFile(cmd.Output, result.Data, 0600); err != nil {
			return c.mapError(fmt.Errorf("write export: %w", err))
		}
		fmt.Fprintf(c.stderr, "wrote tracking metadata export to %s (%d triage, %d contributions, %d outcomes)\n",
			cmd.Output, result.TriageEvents, result.Contributions, result.ContributionOutcomes)
		return nil
	}
	if _, err := c.stdout.Write(result.Data); err != nil {
		return c.mapError(err)
	}
	if len(result.Data) == 0 || result.Data[len(result.Data)-1] != '\n' {
		_, _ = fmt.Fprintln(c.stdout)
	}
	return nil
}

func (c *CLI) runTrackingImport(ctx context.Context, cmd *trackingImportCmd, service TrackingService) error {
	data, err := c.readMetadataImport(cmd.File)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	fmt.Fprintln(c.stderr, "importing local tracking metadata...")
	result, err := service.ImportLocalMetadata(ctx, MetadataImportOptions{Data: data})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) readMetadataImport(file string) ([]byte, error) {
	var reader io.Reader
	if strings.TrimSpace(file) == "" || file == "-" {
		reader = c.stdin
	} else {
		opened, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open import file: %w", err)
		}
		defer opened.Close()
		reader = opened
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxMetadataImportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata import: %w", err)
	}
	if len(data) > maxMetadataImportBytes {
		return nil, fmt.Errorf("metadata import exceeds %s", maxMetadataImportBytesHint)
	}
	return data, nil
}
