package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type prepareCmd struct {
	Issue  issueCmd  `cmd:"" help:"Prepare an issue draft"`
	PR     prCmd     `cmd:"" name:"pr" help:"Prepare a pull request draft"`
	Review reviewCmd `cmd:"" help:"Prepare a read-only review report"`
}

type reviewCmd struct {
	OpportunityID string `arg:"" optional:"" help:"Opportunity ID"`
	WorkspaceID   string `name:"workspace" help:"Workspace ID"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type issueCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	Guidance      string `name:"guidance" help:"Repository contribution guidance"`
	Success       string `name:"success" help:"Success criteria"`
	ManifestID    string `name:"manifest-id" help:"Stored evidence manifest to reference"`
	OutputDir     string `name:"output-dir" type:"path" help:"Atomically export title.txt, body.md, and draft.json"`
	AllowWarnings bool   `name:"allow-warnings" help:"Export when exact-byte validation reports warnings; errors still block export"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type prCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	WorkspaceID   string `name:"workspace" help:"Workspace ID to include diff as changes"`
	Approach      string `name:"approach" required:"" help:"Approach description"`
	Changes       string `name:"changes" help:"Focused changes description"`
	Compatibility string `name:"compatibility" help:"Compatibility notes"`
	Limitations   string `name:"limitations" help:"Limitations"`
	LinkedIssue   string `name:"linked-issue" help:"Linked issue"`
	Guidance      string `name:"guidance" help:"Repository contribution guidance"`
	ManifestID    string `name:"manifest-id" help:"Stored evidence manifest to reference"`
	OutputDir     string `name:"output-dir" type:"path" help:"Atomically export title.txt, body.md, and draft.json"`
	AllowWarnings bool   `name:"allow-warnings" help:"Export when exact-byte validation reports warnings; errors still block export"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) runPrepare(ctx context.Context, command string, cmd *prepareCmd) error {
	service, err := c.contributionService()
	if err != nil {
		return err
	}
	switch command {
	case "prepare issue":
		_, _ = fmt.Fprintf(c.stderr, "preparing issue draft for opportunity %s...\n", cmd.Issue.OpportunityID)
		result, err := service.PrepareIssue(ctx, cmd.Issue.OpportunityID, contracts.PrepareIssueOptions{
			Guidance: cmd.Issue.Guidance, Success: cmd.Issue.Success, ManifestID: cmd.Issue.ManifestID,
		})
		if err != nil {
			return c.mapError(err)
		}
		if err := exportDraft(cmd.Issue.OutputDir, result, cmd.Issue.AllowWarnings); err != nil {
			return err
		}
		return c.render(cmd.Issue.JSON, result)
	case "prepare pr":
		_, _ = fmt.Fprintf(c.stderr, "preparing pull request draft for opportunity %s...\n", cmd.PR.OpportunityID)
		result, err := service.PreparePullRequest(ctx, cmd.PR.OpportunityID, contracts.PreparePROptions{
			WorkspaceID:   cmd.PR.WorkspaceID,
			Approach:      cmd.PR.Approach,
			Changes:       cmd.PR.Changes,
			Compatibility: cmd.PR.Compatibility,
			Limitations:   cmd.PR.Limitations,
			LinkedIssue:   cmd.PR.LinkedIssue,
			Guidance:      cmd.PR.Guidance,
			ManifestID:    cmd.PR.ManifestID,
		})
		if err != nil {
			return c.mapError(err)
		}
		if err := exportDraft(cmd.PR.OutputDir, result, cmd.PR.AllowWarnings); err != nil {
			return err
		}
		return c.render(cmd.PR.JSON, result)
	case "prepare review":
		workflow, err := c.workflowService()
		if err != nil {
			return err
		}
		result, err := workflow.PrepareReviewReport(ctx, contracts.PrepareReviewReportInput{
			OpportunityID: cmd.Review.OpportunityID,
			WorkspaceID:   cmd.Review.WorkspaceID,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Review.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown prepare command: %s", command))
	}
}

func exportDraft(dir string, draft *contracts.DraftResult, allowWarnings bool) error {
	if dir == "" {
		return nil
	}
	if draft == nil {
		return errors.New("cannot export an empty draft")
	}
	for _, finding := range draft.Warnings {
		switch finding.Severity {
		case "error":
			return fmt.Errorf("draft validation error %s: %s", finding.Code, finding.Message)
		case "warning":
			if !allowWarnings {
				return fmt.Errorf("draft validation warning %s: %s (use --allow-warnings to export unchanged bytes)", finding.Code, finding.Message)
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create draft export directory: %w", err)
	}
	metadata, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return fmt.Errorf("encode draft metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	files := []struct {
		name    string
		payload []byte
	}{
		{name: "title.txt", payload: []byte(draft.Title)},
		{name: "body.md", payload: []byte(draft.Body)},
		{name: "draft.json", payload: metadata},
	}
	for _, file := range files {
		if err := atomicWriteDraftFile(filepath.Join(dir, file.name), file.payload); err != nil {
			return err
		}
	}
	return nil
}

func atomicWriteDraftFile(path string, payload []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitcontribute-draft-*")
	if err != nil {
		return fmt.Errorf("create temporary draft file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set draft file permissions: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write draft file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync draft file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close draft file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace draft file: %w", err)
	}
	return nil
}
