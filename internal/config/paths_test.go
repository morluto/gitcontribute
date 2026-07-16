package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPathsDirs(t *testing.T) {
	tests := []struct {
		name    string
		os      string
		env     *Env
		want    map[string]string
		wantErr bool
	}{
		{
			name: "linux with xdg",
			os:   "linux",
			env: &Env{
				Home: "/home/u",
				Vars: map[string]string{
					"XDG_CONFIG_HOME": "/home/u/.xdg/config",
					"XDG_DATA_HOME":   "/home/u/.xdg/share",
					"XDG_CACHE_HOME":  "/home/u/.xdg/cache",
					"XDG_STATE_HOME":  "/home/u/.xdg/state",
				},
			},
			want: map[string]string{
				"config": "/home/u/.xdg/config/gitcontribute",
				"data":   "/home/u/.xdg/share/gitcontribute",
				"cache":  "/home/u/.xdg/cache/gitcontribute",
				"state":  "/home/u/.xdg/state/gitcontribute",
				"log":    "/home/u/.xdg/state/gitcontribute/logs",
			},
		},
		{
			name: "linux fallback",
			os:   "linux",
			env: &Env{
				Home: "/home/u",
				Vars: map[string]string{},
			},
			want: map[string]string{
				"config": "/home/u/.config/gitcontribute",
				"data":   "/home/u/.local/share/gitcontribute",
				"cache":  "/home/u/.cache/gitcontribute",
				"state":  "/home/u/.local/state/gitcontribute",
				"log":    "/home/u/.local/state/gitcontribute/logs",
			},
		},
		{
			name: "macos",
			os:   "darwin",
			env: &Env{
				Home: "/Users/u",
				Vars: map[string]string{},
			},
			want: map[string]string{
				"config": "/Users/u/Library/Application Support/gitcontribute",
				"data":   "/Users/u/Library/Application Support/gitcontribute/Data",
				"cache":  "/Users/u/Library/Caches/gitcontribute",
				"state":  "/Users/u/Library/Application Support/gitcontribute/State",
				"log":    "/Users/u/Library/Logs/gitcontribute",
			},
		},
		{
			name: "windows",
			os:   "windows",
			env: &Env{
				Home: "/win/home/u",
				Vars: map[string]string{
					"APPDATA":      "/win/home/u/AppData/Roaming",
					"LOCALAPPDATA": "/win/home/u/AppData/Local",
				},
			},
			want: map[string]string{
				"config": "/win/home/u/AppData/Roaming/gitcontribute",
				"data":   "/win/home/u/AppData/Local/gitcontribute/Data",
				"cache":  "/win/home/u/AppData/Local/Cache/gitcontribute",
				"state":  "/win/home/u/AppData/Local/gitcontribute/State",
				"log":    "/win/home/u/AppData/Local/gitcontribute/Logs",
			},
		},
		{
			name:    "missing home",
			os:      "linux",
			env:     &Env{Home: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Paths{OS: tt.os, Name: "gitcontribute", Env: tt.env}
			got, err := dirsMap(p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error status: %v", err)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("dir mismatch\ngot:  %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func dirsMap(p *Paths) (map[string]string, error) {
	m := make(map[string]string)
	dirs := map[string]func() (string, error){
		"config": p.ConfigDir,
		"data":   p.DataDir,
		"cache":  p.CacheDir,
		"state":  p.StateDir,
		"log":    p.LogDir,
	}
	for k, fn := range dirs {
		v, err := fn()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}

func TestPathsConfigFileAndDatabasePath(t *testing.T) {
	p := &Paths{OS: "linux", Name: "gitcontribute", Env: &Env{
		Home: "/home/u",
		Vars: map[string]string{"XDG_CONFIG_HOME": "/home/u/.cfg", "XDG_DATA_HOME": "/home/u/.data"},
	}}

	cfg, err := p.ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile error: %v", err)
	}
	want := filepath.Join("/home/u/.cfg", "gitcontribute", "config.toml")
	if cfg != want {
		t.Fatalf("ConfigFile = %q, want %q", cfg, want)
	}

	db, err := p.DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath error: %v", err)
	}
	want = filepath.Join("/home/u/.data", "gitcontribute", "gitcontribute.db")
	if db != want {
		t.Fatalf("DatabasePath = %q, want %q", db, want)
	}
}

func TestPathsDefaultName(t *testing.T) {
	p := &Paths{OS: "linux", Env: &Env{Home: "/home/u", Vars: map[string]string{}}}
	got, err := p.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir error: %v", err)
	}
	want := filepath.Join("/home/u", ".config", appName)
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
}
