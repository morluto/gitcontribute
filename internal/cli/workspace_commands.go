package cli

type workspaceCmd struct {
	Create createWorkspaceCmd `cmd:"" help:"Create a workspace for an investigation"`
	Adopt  adoptWorkspaceCmd  `cmd:"" help:"Adopt an existing local Git worktree"`
	Show   showWorkspaceCmd   `cmd:"" help:"Show a workspace"`
}

type createWorkspaceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Remote          string `name:"remote" help:"Git remote URL (defaults to https://github.com/OWNER/REPO.git)"`
	Base            string `name:"base" help:"Base ref (defaults to remote HEAD)"`
	Candidate       string `name:"candidate" help:"Candidate ref (defaults to investigation commit)"`
	Name            string `name:"name" help:"Workspace name (defaults to generated ID)"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type showWorkspaceCmd struct {
	ID   string `arg:"" help:"Workspace ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type adoptWorkspaceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Path            string `name:"path" required:"" help:"Existing worktree root" type:"path"`
	Base            string `name:"base" required:"" help:"Base ref already available in the repository"`
	Name            string `name:"name" help:"Workspace name (defaults to generated ID)"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type diffCmd struct {
	WorkspaceID string `arg:"" help:"Workspace ID"`
	JSON        bool   `name:"json" help:"Print the result as JSON"`
}
