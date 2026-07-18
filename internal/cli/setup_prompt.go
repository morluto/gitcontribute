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
	SetupQuestionInstall SetupPromptQuestion = "install"
	SetupQuestionClients SetupPromptQuestion = "clients"
	SetupQuestionAuth    SetupPromptQuestion = "auth"
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
	fields := make([]huh.Field, 0, 3)
	if request.asks(SetupQuestionInstall) {
		selection.InstallCLI = true
		fields = append(fields, huh.NewConfirm().
			Title("Install the terminal app?").
			Description("Run gitcontribute directly, including the interactive TUI.").
			Affirmative("Yes, install it").
			Negative("No, use npx").
			Value(&selection.InstallCLI))
	}
	if request.asks(SetupQuestionClients) {
		if len(selection.Clients) == 0 {
			for _, client := range request.Discovery.Clients {
				if client.Detected {
					selection.Clients = append(selection.Clients, client.Name)
				}
			}
		}
		options := make([]huh.Option[string], 0, len(request.Discovery.Clients))
		selected := make(map[string]bool, len(selection.Clients))
		for _, client := range selection.Clients {
			selected[client] = true
		}
		for _, client := range request.Discovery.Clients {
			label := setupClientLabel(client)
			options = append(options, huh.NewOption(label, client.Name).Selected(selected[client.Name]))
		}
		fields = append(fields, huh.NewMultiSelect[string]().
			Title("Configure agent access").
			Description("Space toggles a client. Leave all unchecked for terminal-only setup.").
			Options(options...).
			Value(&selection.Clients))
	}
	if request.asks(SetupQuestionAuth) {
		selection.TokenSource = defaultSetupTokenSource(request.Discovery)
		fields = append(fields, huh.NewSelect[string]().
			Title("GitHub authentication").
			Description("Setup stays local; this only records how later sync commands obtain credentials.").
			Options(setupAuthOptions(request.Discovery)...).
			Value(&selection.TokenSource))
	}
	if len(fields) > 0 {
		_, _ = fmt.Fprintln(p.output, "┌ GitContribute setup")
		form := p.form(fields...)
		if err := form.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
				return SetupSelection{}, ErrSetupCancelled
			}
			return SetupSelection{}, err
		}
	}
	if request.asks(SetupQuestionAuth) {
		selection.TokenSourceKey = setupTokenSourceKey(request, selection.TokenSource)
	}
	if request.asks(SetupQuestionAuth) && (selection.TokenSource == "env" || selection.TokenSource == "keyring") {
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
		form := p.form(field)
		if err := form.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
				return SetupSelection{}, ErrSetupCancelled
			}
			return SetupSelection{}, err
		}
	}
	return selection, nil
}

func (p *huhSetupPrompter) Confirm(ctx context.Context, title string) (bool, error) {
	confirmed := true
	field := huh.NewConfirm().
		Title(title).
		Affirmative("Apply changes").
		Negative("Cancel").
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
	form := huh.NewForm(huh.NewGroup(fields...)).WithInput(p.input).WithOutput(p.output).WithShowHelp(true)
	if os.Getenv("GITCONTRIBUTE_ACCESSIBLE") != "" || os.Getenv("TERM") == "dumb" {
		form.WithAccessible(true)
	}
	return form
}

func setupClientLabel(client SetupClientDiscovery) string {
	name := map[string]string{"codex": "Codex", "claude": "Claude Code"}[client.Name]
	if name == "" {
		name = client.Name
	}
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
	return fmt.Sprintf("%-12s %s  %s", name, state, client.Path)
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
	gh := "GitHub CLI      use the local gh credential helper"
	if discovery.GitHubCLIAvailable {
		gh += " · available"
	}
	envKey := discovery.ConfiguredTokenKey
	if envKey == "" || discovery.ConfiguredTokenSource != "env" {
		envKey = "GITHUB_TOKEN"
	}
	env := "Environment     read " + envKey + " when a sync runs"
	if discovery.EnvironmentKeyPresent {
		env += " · variable present"
	}
	keyring := "System keyring  retrieve a named local entry"
	if discovery.ConfiguredTokenSource == "keyring" {
		keyring += " · configured"
	}
	return []huh.Option[string]{
		huh.NewOption(gh, "gh-cli"),
		huh.NewOption(env, "env"),
		huh.NewOption(keyring, "keyring"),
		huh.NewOption("Configure later use offline features now", "none"),
	}
}
