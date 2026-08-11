package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/pelletier/go-toml/v2"
)

// clientAdapter owns every operation whose representation is specific to one
// coding client. The setup workflow dispatches through this descriptor instead
// of repeating client switches for check, edit, snapshot, and rollback.
type clientAdapter struct {
	client       Client
	path         func(string) string
	detect       func(string) bool
	check        func([]byte) (bool, error)
	configure    func(string, Operation, Launcher, bool) (ChangeStatus, error)
	snapshotData func([]byte) (registrationState, error)
	read         func([]byte) (Launcher, error)
}

var clientAdapters = []*clientAdapter{
	{
		client:       Codex,
		path:         codexConfigPath,
		detect:       detectCodex,
		check:        checkCodexRegistration,
		configure:    editCodex,
		snapshotData: snapshotCodexRegistration,
		read:         readCodexCommand,
	},
	{
		client:       Claude,
		path:         claudeConfigPath,
		detect:       detectClaude,
		check:        checkJSONRegistration,
		configure:    editJSONRegistration,
		snapshotData: snapshotJSONRegistration,
		read:         readJSONCommand,
	},
	{
		client:       Devin,
		path:         devinConfigPath,
		detect:       detectDevin,
		check:        checkJSONRegistration,
		configure:    editJSONRegistration,
		snapshotData: snapshotJSONRegistration,
		read:         readJSONCommand,
	},
}

var allClients = adapterClients()

// SupportedClients returns supported adapters in deterministic application
// order without exposing the package-owned catalog for mutation.
func SupportedClients() []Client {
	return append([]Client(nil), allClients...)
}

func adapterClients() []Client {
	clients := make([]Client, 0, len(clientAdapters))
	for _, adapter := range clientAdapters {
		clients = append(clients, adapter.client)
	}
	return clients
}

func clientAdapterFor(client Client) (*clientAdapter, error) {
	for _, adapter := range clientAdapters {
		if adapter.client == client {
			return adapter, nil
		}
	}
	return nil, fmt.Errorf("unsupported setup client %q", client)
}

func codexConfigPath(home string) string {
	return filepath.Join(home, ".codex", "config.toml")
}

func claudeConfigPath(home string) string {
	return filepath.Join(home, ".claude.json")
}

func devinConfigPath(home string) string {
	appData := ""
	if userHome, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == filepath.Clean(userHome) {
		appData = os.Getenv("APPDATA")
	}
	return devinConfigPathForOS(home, runtime.GOOS, appData)
}

func devinConfigPathForOS(home, goos, appData string) string {
	if goos == "windows" {
		if appData != "" {
			return filepath.Join(appData, "devin", "mcp_config.json")
		}
		return filepath.Join(home, "AppData", "Roaming", "devin", "mcp_config.json")
	}
	return filepath.Join(home, ".config", "devin", "mcp_config.json")
}

func detectCodex(home string) bool {
	return exists(filepath.Dir(codexConfigPath(home)))
}

func detectClaude(home string) bool {
	return exists(filepath.Join(home, ".claude")) || exists(claudeConfigPath(home))
}

func detectDevin(home string) bool {
	return exists(filepath.Dir(devinConfigPath(home)))
}

func checkCodexRegistration(data []byte) (bool, error) {
	_, _, present := findCodexBlock(string(data))
	return present, nil
}

func checkJSONRegistration(data []byte) (bool, error) {
	root, err := parseJSONObject(data, "client config")
	if err != nil {
		return false, err
	}
	rawServers, present := root["mcpServers"]
	if !present {
		return false, nil
	}
	servers, err := parseJSONObject(rawServers, "mcpServers")
	if err != nil {
		return false, errors.New("mcpServers must be an object in claude config")
	}
	rawServer, present := servers[serverName]
	if !present {
		return false, nil
	}
	if _, err := parseJSONObject(rawServer, "gitcontribute server"); err != nil {
		return false, errors.New("gitcontribute server must be an object in claude config")
	}
	return true, nil
}

func snapshotCodexRegistration(data []byte) (registrationState, error) {
	start, end, present := findCodexBlock(string(data))
	if !present {
		return nil, errors.New("codex registration disappeared before activation")
	}
	return codexRegistrationState{block: string(data[start:end])}, nil
}

func snapshotJSONRegistration(data []byte) (registrationState, error) {
	root, err := parseJSONObject(data, "client registration snapshot")
	if err != nil {
		return nil, fmt.Errorf("parse client registration snapshot: %w", err)
	}
	servers, err := parseJSONObject(root["mcpServers"], "mcpServers")
	if err != nil {
		return nil, errors.New("client mcpServers disappeared before activation")
	}
	entry, present := servers[serverName]
	if !present {
		return nil, errors.New("GitContribute server disappeared before activation")
	}
	return jsonRegistrationState{entry: append(json.RawMessage(nil), entry...)}, nil
}

func registrationFileMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

// ReadCommand reads the durable launcher stored in a client-owned config.
// Parsing remains inside the client adapter; callers receive the product-owned
// Launcher contract rather than a client-specific representation.
func ReadCommand(client Client, home string) (Launcher, error) {
	adapter, err := clientAdapterFor(client)
	if err != nil {
		return Launcher{}, err
	}
	return readCommandFile(client, adapter.path(home))
}

func readCommandFile(client Client, path string) (Launcher, error) {
	adapter, err := clientAdapterFor(client)
	if err != nil {
		return Launcher{}, err
	}
	data, err := readFileWithinParent(path)
	if err != nil {
		return Launcher{}, err
	}
	return adapter.read(data)
}

func readCodexCommand(data []byte) (Launcher, error) {
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `toml:"command"`
			Args    []string `toml:"args"`
		} `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Launcher{}, err
	}
	server, ok := cfg.MCPServers[serverName]
	if !ok {
		return Launcher{}, errors.New("gitcontribute server not found in codex config")
	}
	return Launcher{Command: server.Command, Args: server.Args}, nil
}

func readJSONCommand(data []byte) (Launcher, error) {
	root, err := parseJSONObject(data, "client config")
	if err != nil {
		return Launcher{}, err
	}
	servers, err := parseJSONObject(root["mcpServers"], "mcpServers")
	if err != nil {
		return Launcher{}, errors.New("mcpServers is missing from claude config")
	}
	server, err := parseJSONObject(servers[serverName], "gitcontribute server")
	if err != nil {
		return Launcher{}, errors.New("gitcontribute server not found in claude config")
	}
	return launcherFromJSONObject(server)
}

func launcherFromJSONObject(server jsonObject) (Launcher, error) {
	var command string
	if err := json.Unmarshal(server["command"], &command); err != nil || command == "" {
		return Launcher{}, errors.New("gitcontribute command is missing from claude config")
	}
	var argsIn []json.RawMessage
	if err := json.Unmarshal(server["args"], &argsIn); err != nil {
		return Launcher{}, errors.New("gitcontribute args are missing from claude config")
	}
	args := make([]string, 0, len(argsIn))
	for i, raw := range argsIn {
		var arg string
		if err := json.Unmarshal(raw, &arg); err != nil {
			return Launcher{}, fmt.Errorf("gitcontribute args[%d] must be a string", i)
		}
		args = append(args, arg)
	}
	return Launcher{Command: command, Args: args}, nil
}

type jsonObject map[string]json.RawMessage

func parseJSONObject(data []byte, name string) (jsonObject, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	var object jsonObject
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, err
	}
	return object, nil
}

func exactLauncherEntry(data json.RawMessage, launcher Launcher) bool {
	entry, err := parseJSONObject(data, "GitContribute server")
	if err != nil || len(entry) != 2 {
		return false
	}
	parsed, err := launcherFromJSONObject(entry)
	return err == nil && parsed.Command == launcher.Command && slices.Equal(parsed.Args, launcher.Args)
}
