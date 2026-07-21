package cli

// UpgradeStage reports one inspectable upgrade stage.
type UpgradeStage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message,omitempty"`
}

// UpgradeConfiguredClient reports one coding client's runtime registration.
type UpgradeConfiguredClient struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UpgradeReport describes installation, compatibility, activation, and rollback.
type UpgradeReport struct {
	Context           string                    `json:"context"`
	Current           string                    `json:"current"`
	Latest            string                    `json:"latest,omitempty"`
	Status            string                    `json:"status"`
	Command           string                    `json:"command,omitempty"`
	Action            string                    `json:"action,omitempty"`
	Rollback          string                    `json:"rollback,omitempty"`
	RestartClients    []string                  `json:"restart_clients,omitempty"`
	Stages            []UpgradeStage            `json:"stages"`
	ConfiguredClients []UpgradeConfiguredClient `json:"configured_clients,omitempty"`
}
