package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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
	configure    func(string, Operation, Launcher, bool) (string, error)
	snapshotData func([]byte, *registrationSnapshot) error
	restore      func(registrationSnapshot, Launcher) error
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
		restore:      restoreCodexRegistration,
		read:         readCodexCommand,
	},
	{
		client:       Claude,
		path:         claudeConfigPath,
		detect:       detectClaude,
		check:        checkJSONRegistration,
		configure:    editJSONRegistration,
		snapshotData: snapshotJSONRegistration,
		restore:      restoreJSONRegistration,
		read:         readJSONCommand,
	},
	{
		client:       Devin,
		path:         devinConfigPath,
		detect:       detectDevin,
		check:        checkJSONRegistration,
		configure:    editJSONRegistration,
		snapshotData: snapshotJSONRegistration,
		restore:      restoreJSONRegistration,
		read:         readJSONCommand,
	},
}

// AllClients lists supported adapters in deterministic application order.
var AllClients = adapterClients()

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
	return devinConfigPathForOS(home, runtime.GOOS)
}

func devinConfigPathForOS(home, goos string) string {
	if goos == "windows" {
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
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, err
	}
	rawServers, present := root["mcpServers"]
	if !present {
		return false, nil
	}
	servers, ok := rawServers.(map[string]any)
	if !ok {
		return false, errors.New("mcpServers must be an object in claude config")
	}
	rawServer, present := servers[serverName]
	if !present {
		return false, nil
	}
	if _, ok := rawServer.(map[string]any); !ok {
		return false, errors.New("gitcontribute server must be an object in claude config")
	}
	return true, nil
}

func snapshotCodexRegistration(data []byte, snapshot *registrationSnapshot) error {
	start, end, present := findCodexBlock(string(data))
	if !present {
		return errors.New("codex registration disappeared before activation")
	}
	snapshot.codexBlock = string(data[start:end])
	return nil
}

func snapshotJSONRegistration(data []byte, snapshot *registrationSnapshot) error {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse Claude registration snapshot: %w", err)
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return errors.New("claude mcpServers disappeared before activation")
	}
	snapshot.jsonEntry = servers[serverName]
	return nil
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
	return ReadCommandFile(client, adapter.path(home))
}

// ReadCommandFile is the path-oriented form used by setup-owned tests and
// recovery tooling. The config parser remains owned by the selected adapter.
func ReadCommandFile(client Client, path string) (Launcher, error) {
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
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return Launcher{}, err
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return Launcher{}, errors.New("mcpServers is missing from claude config")
	}
	server, ok := servers[serverName].(map[string]any)
	if !ok {
		return Launcher{}, errors.New("gitcontribute server not found in claude config")
	}
	command, ok := server["command"].(string)
	if !ok || command == "" {
		return Launcher{}, errors.New("gitcontribute command is missing from claude config")
	}
	argsIn, ok := server["args"].([]any)
	if !ok {
		return Launcher{}, errors.New("gitcontribute args are missing from claude config")
	}
	args := make([]string, 0, len(argsIn))
	for i, raw := range argsIn {
		arg, ok := raw.(string)
		if !ok {
			return Launcher{}, fmt.Errorf("gitcontribute args[%d] must be a string", i)
		}
		args = append(args, arg)
	}
	return Launcher{Command: command, Args: args}, nil
}
