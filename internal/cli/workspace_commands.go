package cli

type workspaceCmd struct {
	Create createWorkspaceCmd `cmd:"" help:"Create a workspace for an investigation"`
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

type diffCmd struct {
	WorkspaceID string `arg:"" help:"Workspace ID"`
	JSON        bool   `name:"json" help:"Print the result as JSON"`
}
