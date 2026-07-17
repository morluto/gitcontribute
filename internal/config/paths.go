package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "gitcontribute"

// Env provides an injectable environment for tests. A nil Env uses the real
// process environment and home directory.
type Env struct {
	Home string
	Vars map[string]string
}

// Paths resolves platform-native application directories.
type Paths struct {
	OS   string
	Name string
	Env  *Env
}

// NewPaths returns a Paths resolver for the current platform using env for
// environment injection. If env is nil, the real process environment is used.
func NewPaths(env *Env) *Paths {
	return &Paths{Env: env}
}

func (p *Paths) os() string {
	if p.OS != "" {
		return p.OS
	}
	return runtime.GOOS
}

func (p *Paths) name() string {
	if p.Name != "" {
		return p.Name
	}
	return appName
}

func (p *Paths) getenv(key string) string {
	if p.Env != nil {
		if v, ok := p.Env.Vars[key]; ok {
			return v
		}
		if key == "HOME" && p.Env.Home != "" {
			return p.Env.Home
		}
		return ""
	}
	if key == "HOME" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	return os.Getenv(key)
}

func (p *Paths) homeDir() string {
	if home := p.getenv("HOME"); home != "" {
		return home
	}
	return ""
}

// HomeDir returns the configured user home used for coding-client discovery.
// It honors the injected environment used by tests.
func (p *Paths) HomeDir() string { return p.homeDir() }

func (p *Paths) windowsProfile() string {
	if v := p.getenv("USERPROFILE"); v != "" {
		return v
	}
	if home := p.homeDir(); home != "" {
		return home
	}
	return ""
}

func (p *Paths) configRoot() (string, error) {
	switch p.os() {
	case "windows":
		if v := p.getenv("APPDATA"); v != "" {
			return v, nil
		}
		if prof := p.windowsProfile(); prof != "" {
			return filepath.Join(prof, "AppData", "Roaming"), nil
		}
		return "", errors.New("could not resolve Windows config directory: APPDATA and USERPROFILE are unset")
	case "darwin":
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve macOS config directory: HOME is unset")
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		if v := p.getenv("XDG_CONFIG_HOME"); v != "" {
			return v, nil
		}
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve Linux/Unix config directory: XDG_CONFIG_HOME and HOME are unset")
		}
		return filepath.Join(home, ".config"), nil
	}
}

func (p *Paths) dataRoot() (string, error) {
	switch p.os() {
	case "windows":
		if v := p.getenv("LOCALAPPDATA"); v != "" {
			return v, nil
		}
		if prof := p.windowsProfile(); prof != "" {
			return filepath.Join(prof, "AppData", "Local"), nil
		}
		return "", errors.New("could not resolve Windows data directory: LOCALAPPDATA and USERPROFILE are unset")
	case "darwin":
		return p.configRoot()
	default:
		if v := p.getenv("XDG_DATA_HOME"); v != "" {
			return v, nil
		}
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve Linux/Unix data directory: XDG_DATA_HOME and HOME are unset")
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}

func (p *Paths) cacheRoot() (string, error) {
	switch p.os() {
	case "windows":
		local, err := p.dataRoot()
		if err != nil {
			return "", err
		}
		return filepath.Join(local, "Cache"), nil
	case "darwin":
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve macOS cache directory: HOME is unset")
		}
		return filepath.Join(home, "Library", "Caches"), nil
	default:
		if v := p.getenv("XDG_CACHE_HOME"); v != "" {
			return v, nil
		}
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve Linux/Unix cache directory: XDG_CACHE_HOME and HOME are unset")
		}
		return filepath.Join(home, ".cache"), nil
	}
}

func (p *Paths) stateRoot() (string, error) {
	switch p.os() {
	case "windows":
		return p.dataRoot()
	case "darwin":
		return p.configRoot()
	default:
		if v := p.getenv("XDG_STATE_HOME"); v != "" {
			return v, nil
		}
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve Linux/Unix state directory: XDG_STATE_HOME and HOME are unset")
		}
		return filepath.Join(home, ".local", "state"), nil
	}
}

func (p *Paths) logRoot() (string, error) {
	switch p.os() {
	case "windows":
		return p.dataRoot()
	case "darwin":
		home := p.homeDir()
		if home == "" {
			return "", errors.New("could not resolve macOS log directory: HOME is unset")
		}
		return filepath.Join(home, "Library", "Logs"), nil
	default:
		return p.stateRoot()
	}
}

// ConfigDir returns the platform-native configuration directory for the app.
func (p *Paths) ConfigDir() (string, error) {
	root, err := p.configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, p.name()), nil
}

// DataDir returns the platform-native data directory for the app.
func (p *Paths) DataDir() (string, error) {
	root, err := p.dataRoot()
	if err != nil {
		return "", err
	}
	switch p.os() {
	case "windows", "darwin":
		return filepath.Join(root, p.name(), "Data"), nil
	default:
		return filepath.Join(root, p.name()), nil
	}
}

// AcquisitionCacheDir returns the managed code-acquisition cache directory.
func (p *Paths) AcquisitionCacheDir() (string, error) {
	root, err := p.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "acquisitions"), nil
}

// CacheDir returns the platform-native cache directory for the app.
func (p *Paths) CacheDir() (string, error) {
	root, err := p.cacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, p.name()), nil
}

// StateDir returns the platform-native state directory for the app.
func (p *Paths) StateDir() (string, error) {
	root, err := p.stateRoot()
	if err != nil {
		return "", err
	}
	switch p.os() {
	case "windows", "darwin":
		return filepath.Join(root, p.name(), "State"), nil
	default:
		return filepath.Join(root, p.name()), nil
	}
}

// LogDir returns the platform-native log directory for the app.
func (p *Paths) LogDir() (string, error) {
	root, err := p.logRoot()
	if err != nil {
		return "", err
	}
	switch p.os() {
	case "darwin":
		return filepath.Join(root, p.name()), nil
	case "windows":
		return filepath.Join(root, p.name(), "Logs"), nil
	default:
		return filepath.Join(root, p.name(), "logs"), nil
	}
}

// ConfigFile returns the full path to the configuration file.
func (p *Paths) ConfigFile() (string, error) {
	dir, err := p.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// DatabasePath returns the default database path inside DataDir.
func (p *Paths) DatabasePath() (string, error) {
	dir, err := p.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gitcontribute.db"), nil
}
