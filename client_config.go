package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClientConfig holds user defaults for new session creation.
// Stored at ~/.orbitor/config.json.
//
// Example:
//
//	{
//	  "serverURL": "http://127.0.0.1:8080",
//	  "defaultBackend": "claude",
//	  "defaultModel": "claude-sonnet-4-6",
//	  "skipPermissions": false,
//	  "planMode": false
//	}
type ClientConfig struct {
	ServerURL       string `json:"serverURL"`
	DefaultBackend  string `json:"defaultBackend"`
	DefaultModel    string `json:"defaultModel"`
	SkipPermissions bool   `json:"skipPermissions"`
	PlanMode        bool   `json:"planMode"`
}

// defaultClientConfig returns built-in defaults used when no config file exists.
func defaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerURL:      "http://127.0.0.1:8080",
		DefaultBackend: "copilot",
	}
}

// ClientConfigPath returns the path to ~/.orbitor/config.json.
func ClientConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".orbitor", "config.json"), nil
}

// LoadClientConfig reads ~/.orbitor/config.json and merges it with built-in
// defaults. Missing or unreadable config silently falls back to defaults.
func LoadClientConfig() ClientConfig {
	cfg := defaultClientConfig()
	path, err := ClientConfigPath()
	if err != nil {
		return cfg
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()
	// Decode into cfg so only explicitly-set fields overwrite defaults.
	_ = json.NewDecoder(f).Decode(&cfg)
	if cfg.ServerURL == "" {
		cfg.ServerURL = "http://127.0.0.1:8080"
	}
	if cfg.DefaultBackend == "" {
		cfg.DefaultBackend = "copilot"
	}
	return cfg
}
