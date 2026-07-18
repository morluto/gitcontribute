package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
)

// ErrSetupCancelled reports a deliberate interactive cancellation.
var ErrSetupCancelled = errors.New("setup cancelled")

// SetupPromptRequest contains resolved flags plus the questions that still
// require interactive consent.
type SetupPromptRequest struct {
	Discovery      SetupDiscovery
	Questions      []SetupPromptQuestion
	InstallCLI     bool
	Clients        []string
	TokenSource    string
	TokenSourceKey string
}

// SetupPromptQuestion identifies one unresolved onboarding decision.
type SetupPromptQuestion string

const (
	// SetupQuestionInstall asks whether to install the persistent terminal app.
	SetupQuestionInstall SetupPromptQuestion = "install"
	// SetupQuestionClients asks which detected clients to configure.
	SetupQuestionClients SetupPromptQuestion = "clients"
	// SetupQuestionAuth asks how later sync commands should authenticate.
	SetupQuestionAuth SetupPromptQuestion = "auth"
)

func (r SetupPromptRequest) asks(question SetupPromptQuestion) bool {
	for _, candidate := range r.Questions {
		if candidate == question {
			return true
		}
	}
	return false
}

// SetupSelection is the typed result of the interactive setup form.
type SetupSelection struct {
	InstallCLI     bool
	Clients        []string
	TokenSource    string
	TokenSourceKey string
}

// SetupPrompter owns interactive terminal mechanics. Application services
// never depend on Huh, Bubble Tea, or terminal styling types.
type SetupPrompter interface {
	Select(context.Context, SetupPromptRequest) (SetupSelection, error)
	Confirm(context.Context, string) (bool, error)
}

type huhSetupPrompter struct {
	input  io.Reader
	output io.Writer
}

func newSetupPrompter(input io.Reader, output io.Writer) SetupPrompter {
	return &huhSetupPrompter{input: input, output: output}
}

func (p *huhSetupPrompter) Select(ctx context.Context, request SetupPromptRequest) (SetupSelection, error) {
	selection := SetupSelection{
		InstallCLI:     request.InstallCLI,
		Clients:        append([]string(nil), request.Clients...),
		TokenSource:    request.TokenSource,
		TokenSourceKey: request.TokenSourceKey,
	}
	fields := setupPromptFields(request, &selection)
	if len(fields) > 0 {
		if _, err := fmt.Fprintln(p.output, renderSetupDiscovery(request.Discovery)); err != nil {
			return SetupSelection{}, fmt.Errorf("write setup heading: %w", err)
		}
		if request.asks(SetupQuestionInstall) {
			if _, err := fmt.Fprintln(p.output, "npm allowed this one-time run. This choice controls future runs."); err != nil {
				return SetupSelection{}, fmt.Errorf("write npx explanation: %w", err)
			}
		}
		form := p.form(fields...)
		if err := runSetupForm(ctx, form); err != nil {
			return SetupSelection{}, err
		}
	}
	if request.asks(SetupQuestionAuth) {
		selection.TokenSourceKey = setupTokenSourceKey(request, selection.TokenSource)
	}
	if request.asks(SetupQuestionAuth) {
		if err := p.promptTokenSourceKey(ctx, &selection); err != nil {
			return SetupSelection{}, err
		}
	}
	return selection, nil
}

func setupPromptFields(request SetupPromptRequest, selection *SetupSelection) []huh.Field {
	fields := make([]huh.Field, 0, 3)
	total := len(request.Questions)
	for index, question := range request.Questions {
		title := func(value string) string { return setupStageTitle(index+1, total, value) }
		switch question {
		case SetupQuestionInstall:
			selection.InstallCLI = true
			fields = append(fields, huh.NewSelect[bool]().
				Title(title("How do you want to run GitContribute?")).
				Options(
					huh.NewOption("Install the gitcontribute command  · recommended\n    Run it directly from any terminal.", true),
					huh.NewOption("Keep using npx\n    Nothing is installed globally; commands keep the npx prefix.", false),
				).
				Value(&selection.InstallCLI))
		case SetupQuestionClients:
			fields = append(fields, setupClientsField(title("Use GitContribute from coding agents"), request.Discovery, &selection.Clients))
		case SetupQuestionAuth:
			selection.TokenSource = defaultSetupTokenSource(request.Discovery)
			fields = append(fields, huh.NewSelect[string]().
				Title(title("How should future GitHub syncs authenticate?")).
				Description("Setup stays local and does not contact GitHub or validate credentials.").
				Options(setupAuthOptions(request.Discovery)...).
				Value(&selection.TokenSource))
		}
	}
	return fields
}

func setupClientsField(title string, discovery SetupDiscovery, clients *[]string) huh.Field {
	if len(*clients) == 0 {
		for _, client := range discovery.Clients {
			if client.Detected {
				*clients = append(*clients, client.Name)
			}
		}
	}
	selected := make(map[string]bool, len(*clients))
	for _, client := range *clients {
		selected[client] = true
	}
	options := make([]huh.Option[string], 0, len(discovery.Clients))
	for _, client := range discovery.Clients {
		options = append(options, huh.NewOption(setupClientLabel(client), client.Name).Selected(selected[client.Name]))
	}
	return huh.NewMultiSelect[string]().
		Title(title).
		Description("Select clients for MCP access. Leave all unchecked for terminal-only setup.").
		Options(options...).
		Limit(len(options)).
		Height(len(options)*2 + 2).
		Value(clients)
}

func setupStageTitle(current, total int, title string) string {
	if total < 2 {
		return title
	}
	return fmt.Sprintf("%d of %d · %s", current, total, title)
}

func renderSetupDiscovery(discovery SetupDiscovery) string {
	detected := make([]string, 0, len(discovery.Clients)+1)
	for _, client := range discovery.Clients {
		if client.Detected {
			detected = append(detected, setupClientName(client.Name))
		}
	}
	if discovery.GitHubCLIAvailable {
		detected = append(detected, "GitHub CLI")
	}
	summary := "No supported coding agents detected"
	if len(detected) > 0 {
		summary = "Detected " + strings.Join(detected, ", ")
	}
	return "┌ GitContribute setup\n│ " + summary + "\n└ Local inspection only · no changes made"
}

func (p *huhSetupPrompter) promptTokenSourceKey(ctx context.Context, selection *SetupSelection) error {
	if selection.TokenSource != "env" && selection.TokenSource != "keyring" {
		return nil
	}
	if selection.TokenSource == "env" && selection.TokenSourceKey == "" {
		selection.TokenSourceKey = "GITHUB_TOKEN"
	}
	title := "Environment variable name"
	description := "Only the variable name is stored; its value is never written to configuration."
	if selection.TokenSource == "keyring" {
		title = "System keyring entry"
		description = "Store the entry name, not a token."
	}
	field := huh.NewInput().Title(title).Description(description).Value(&selection.TokenSourceKey).
		Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("a name is required")
			}
			return nil
		})
	return runSetupForm(ctx, p.form(field))
}

func runSetupForm(ctx context.Context, form *huh.Form) error {
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
			return ErrSetupCancelled
		}
		return err
	}
	return nil
}

func (p *huhSetupPrompter) Confirm(ctx context.Context, title string) (bool, error) {
	confirmed := true
	field := huh.NewSelect[bool]().
		Title(title).
		Description("Review the exact paths and commands above before continuing.").
		Options(
			huh.NewOption("Apply changes", true),
			huh.NewOption("Cancel  · no changes will be made", false),
		).
		Value(&confirmed)
	form := p.form(field)
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
			return false, ErrSetupCancelled
		}
		return false, err
	}
	return confirmed, nil
}

func (p *huhSetupPrompter) form(fields ...huh.Field) *huh.Form {
	groups := setupPromptGroups(fields)
	plain := os.Getenv("GITCONTRIBUTE_ACCESSIBLE") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != ""
	theme := huh.ThemeFunc(huh.ThemeBase16)
	if plain {
		theme = huh.ThemeFunc(huh.ThemeBase)
	}
	keymap := huh.NewDefaultKeyMap()
	keymap.Select.Filter.SetEnabled(false)
	keymap.MultiSelect.Filter.SetEnabled(false)
	keymap.MultiSelect.SelectAll.SetEnabled(false)
	keymap.MultiSelect.SelectNone.SetEnabled(false)
	keymap.MultiSelect.Toggle.SetHelp("space", "toggle")
	form := huh.NewForm(groups...).WithInput(p.input).WithOutput(p.output).WithShowHelp(true).WithTheme(theme).WithKeyMap(keymap)
	if os.Getenv("GITCONTRIBUTE_ACCESSIBLE") != "" || os.Getenv("TERM") == "dumb" {
		form.WithAccessible(true)
	}
	return form
}

func setupPromptGroups(fields []huh.Field) []*huh.Group {
	groups := make([]*huh.Group, 0, len(fields))
	for _, field := range fields {
		// Groups are pages in Huh's default layout. Keeping one field in each
		// group guarantees that completed controls and their key bindings do
		// not compete with the active setup decision.
		groups = append(groups, huh.NewGroup(field))
	}
	return groups
}

func setupClientLabel(client SetupClientDiscovery) string {
	name := setupClientName(client.Name)
	state := "not detected"
	if client.Detected {
		state = "detected"
	}
	if client.Registered {
		state += " · configured"
	}
	if client.Error != "" {
		state += " · config needs attention"
	}
	return fmt.Sprintf("%-12s %s\n    %s", name, state, client.Path)
}

func setupClientName(name string) string {
	if display := map[string]string{"codex": "Codex", "claude": "Claude Code"}[name]; display != "" {
		return display
	}
	return name
}

func defaultSetupTokenSource(discovery SetupDiscovery) string {
	switch discovery.ConfiguredTokenSource {
	case "gh-cli", "env", "keyring":
		return discovery.ConfiguredTokenSource
	}
	if discovery.GitHubCLIAvailable {
		return "gh-cli"
	}
	if discovery.EnvironmentKeyPresent {
		return "env"
	}
	return "none"
}

func setupTokenSourceKey(request SetupPromptRequest, selectedSource string) string {
	if request.TokenSourceKey != "" {
		return request.TokenSourceKey
	}
	if selectedSource == request.Discovery.ConfiguredTokenSource {
		return request.Discovery.ConfiguredTokenKey
	}
	if selectedSource == "env" {
		return "GITHUB_TOKEN"
	}
	return ""
}

func setupAuthOptions(discovery SetupDiscovery) []huh.Option[string] {
	gh := "GitHub CLI"
	if discovery.GitHubCLIAvailable {
		gh += " · available"
	}
	gh += "\n    Use the local gh credential helper; store no token."
	envKey := discovery.ConfiguredTokenKey
	if envKey == "" || discovery.ConfiguredTokenSource != "env" {
		envKey = "GITHUB_TOKEN"
	}
	env := "Environment variable"
	if discovery.EnvironmentKeyPresent {
		env += " · present"
	}
	env += "\n    Read " + envKey + " when a sync runs; store only its name."
	keyring := "System keyring"
	if discovery.ConfiguredTokenSource == "keyring" {
		keyring += " · configured"
	}
	keyring += "\n    Retrieve a named local entry; store only the entry name."
	return []huh.Option[string]{
		huh.NewOption(gh, "gh-cli"),
		huh.NewOption(env, "env"),
		huh.NewOption(keyring, "keyring"),
		huh.NewOption("Configure later\n    Use offline features now and choose before the first sync.", "none"),
	}
}
