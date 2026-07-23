package cli

type exportCmd struct {
	Dossier  exportDossierCmd  `cmd:"" help:"Export a repository dossier"`
	Evidence exportEvidenceCmd `cmd:"" help:"Export investigation evidence"`
	Manifest exportManifestCmd `cmd:"" help:"Export a contribution evidence manifest"`
}

type exportDossierCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Format    string `name:"format" default:"markdown" enum:"json,markdown,md" help:"Export format"`
	Output    string `name:"output" help:"Write to a file instead of stdout"`
}

type exportEvidenceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Format          string `name:"format" default:"markdown" enum:"json,markdown,md" help:"Export format"`
	Output          string `name:"output" help:"Write to a file instead of stdout"`
}

type exportManifestCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	WorkspaceID   string `name:"workspace" help:"Managed workspace ID to bind"`
	PullRequest   string `name:"pull-request" help:"Exact stored pull request as OWNER/REPO#NUMBER"`
	Output        string `name:"output" help:"Write to a file instead of stdout"`
}
