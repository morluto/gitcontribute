package cli

import (
	"context"
	"fmt"
)

type concernCmd struct {
	Create  createConcernCmd  `cmd:"" help:"Record a local concern"`
	List    listConcernCmd    `cmd:"" help:"List or search local concerns"`
	Show    showConcernCmd    `cmd:"" help:"Show a local concern"`
	Update  updateConcernCmd  `cmd:"" help:"Update a local concern"`
	Status  statusConcernCmd  `cmd:"" help:"Change concern lifecycle status"`
	Link    linkConcernCmd    `cmd:"" help:"Link a concern to related local evidence"`
	Promote promoteConcernCmd `cmd:"" help:"Promote a concern into contribution workflow"`
}

type createConcernCmd struct {
	OwnerRepo  string   `arg:"" help:"Repository as OWNER/REPO"`
	Title      string   `name:"title" required:"" help:"Concise concern title"`
	Problem    string   `name:"problem" required:"" help:"Problem statement"`
	Commit     string   `name:"commit" help:"Source commit SHA"`
	Workspace  string   `name:"workspace" help:"Opaque workspace ID"`
	Owner      string   `name:"suspected-owner" help:"Suspected code ownership boundary"`
	Confidence float64  `name:"confidence" help:"Confidence from 0 to 1"`
	Unknowns   []string `name:"unknown" help:"Explicit unknown (repeatable)"`
	Success    string   `name:"success" help:"Proof or success criterion"`
	Notes      string   `name:"notes" help:"Local notes"`
	Evidence   []string `name:"evidence" help:"Evidence ID (repeatable)"`
	JSON       bool     `name:"json" help:"Print the result as JSON"`
}

type listConcernCmd struct {
	OwnerRepo string `arg:"" optional:"" help:"Optional repository as OWNER/REPO"`
	Query     string `name:"query" help:"Literal full-text query"`
	Status    string `name:"status" help:"Concern status"`
	Limit     int    `name:"limit" default:"20" help:"Maximum results (1-100)"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type showConcernCmd struct {
	ID   string `arg:"" help:"Concern ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type updateConcernCmd struct {
	ID         string   `arg:"" help:"Concern ID"`
	Title      *string  `name:"title" help:"Replacement title"`
	Problem    *string  `name:"problem" help:"Replacement problem statement"`
	Owner      *string  `name:"suspected-owner" help:"Replacement owner boundary"`
	Confidence *float64 `name:"confidence" help:"Replacement confidence"`
	Unknowns   []string `name:"unknown" help:"Replacement unknowns"`
	Success    *string  `name:"success" help:"Replacement success criterion"`
	Notes      *string  `name:"notes" help:"Replacement local notes"`
	Evidence   []string `name:"evidence" help:"Replacement evidence IDs"`
	JSON       bool     `name:"json" help:"Print the result as JSON"`
}

type statusConcernCmd struct {
	ID        string `arg:"" help:"Concern ID"`
	Status    string `arg:"" enum:"untriaged,accepted,investigating,deferred,promoted,resolved" help:"Target status"`
	Rationale string `name:"rationale" required:"" help:"Reason for transition"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type linkConcernCmd struct {
	ID         string `arg:"" help:"Concern ID"`
	Kind       string `name:"kind" required:"" enum:"related,duplicate_candidate,hotspot,investigation,opportunity" help:"Relationship kind"`
	TargetType string `name:"target-type" required:"" help:"Target record type"`
	TargetID   string `name:"target" required:"" help:"Target record ID"`
	Note       string `name:"note" help:"Relationship note"`
	JSON       bool   `name:"json" help:"Print the result as JSON"`
}

type promoteConcernCmd struct {
	ID       string `arg:"" help:"Concern ID"`
	Kind     string `name:"kind" required:"" enum:"investigation,opportunity" help:"Promotion target"`
	Category string `name:"category" required:"" enum:"bug,performance,architecture,testing,documentation,maintenance,compatibility,security,other" help:"Contribution category"`
	Scope    string `name:"scope" help:"Opportunity scope"`
	Impact   string `name:"impact" help:"Opportunity impact"`
	Effort   string `name:"effort" help:"Expected effort"`
	JSON     bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) concernService() (ConcernService, error) {
	service, ok := c.svc.(ConcernService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runConcern(ctx context.Context, command string, cmd *concernCmd) error {
	service, err := c.concernService()
	if err != nil {
		return err
	}
	var result any
	var jsonOutput bool
	switch command {
	case "concern create":
		repo, parseErr := parseRepo(cmd.Create.OwnerRepo)
		if parseErr != nil {
			return parseErr
		}
		result, err = service.CreateConcern(ctx, ConcernCreateOptions{
			Repo: repo, CommitSHA: cmd.Create.Commit, WorkspaceID: cmd.Create.Workspace,
			Title: cmd.Create.Title, ProblemStatement: cmd.Create.Problem, SuspectedOwner: cmd.Create.Owner,
			Confidence: cmd.Create.Confidence, Unknowns: cmd.Create.Unknowns, SuccessCriterion: cmd.Create.Success,
			Notes: cmd.Create.Notes, EvidenceIDs: cmd.Create.Evidence,
		})
		jsonOutput = cmd.Create.JSON
	case "concern list":
		var repo RepoRef
		if cmd.List.OwnerRepo != "" {
			repo, err = parseRepo(cmd.List.OwnerRepo)
			if err != nil {
				return err
			}
		}
		result, err = service.ListConcerns(ctx, ConcernListOptions{Repo: repo, Status: cmd.List.Status, Query: cmd.List.Query, Limit: cmd.List.Limit})
		jsonOutput = cmd.List.JSON
	case "concern show":
		result, err = service.ShowConcern(ctx, cmd.Show.ID)
		jsonOutput = cmd.Show.JSON
	case "concern update":
		result, err = service.UpdateConcern(ctx, cmd.Update.ID, ConcernUpdateOptions{
			Title: cmd.Update.Title, ProblemStatement: cmd.Update.Problem, SuspectedOwner: cmd.Update.Owner,
			Confidence: cmd.Update.Confidence, Unknowns: cmd.Update.Unknowns, SuccessCriterion: cmd.Update.Success,
			Notes: cmd.Update.Notes, EvidenceIDs: cmd.Update.Evidence,
		})
		jsonOutput = cmd.Update.JSON
	case "concern status":
		result, err = service.SetConcernStatus(ctx, cmd.Status.ID, cmd.Status.Status, cmd.Status.Rationale)
		jsonOutput = cmd.Status.JSON
	case "concern link":
		result, err = service.LinkConcern(ctx, cmd.Link.ID, ConcernLinkOptions{Kind: cmd.Link.Kind, TargetType: cmd.Link.TargetType, TargetID: cmd.Link.TargetID, Note: cmd.Link.Note})
		jsonOutput = cmd.Link.JSON
	case "concern promote":
		result, err = service.PromoteConcern(ctx, cmd.Promote.ID, ConcernPromoteOptions{Kind: cmd.Promote.Kind, Category: cmd.Promote.Category, Scope: cmd.Promote.Scope, Impact: cmd.Promote.Impact, ExpectedEffort: cmd.Promote.Effort})
		jsonOutput = cmd.Promote.JSON
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown concern command: %s", command))
	}
	if err != nil {
		return c.mapError(err)
	}
	return c.render(jsonOutput, result)
}
