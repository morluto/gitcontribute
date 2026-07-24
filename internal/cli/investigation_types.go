package cli

type investigationCmd struct {
	Start       startInvestigationCmd       `cmd:"" help:"Start an investigation"`
	StartThread startThreadInvestigationCmd `cmd:"" name:"start-thread" help:"Atomically start from one stored issue or pull request"`
	Show        showInvestigationCmd        `cmd:"" help:"Show an investigation"`
	List        listInvestigationCmd        `cmd:"" help:"List investigations"`
}

// InvestigationResult is a single investigation view.

// HypothesisResult is a single hypothesis view.
