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
//	  "listenAddr": "127.0.0.1:8080",
//	  "defaultBackend": "claude",
//	  "defaultModel": "claude-sonnet-4-6",
//	  "skipPermissions": false,
//	  "planMode": false
//	}
type ClientConfig struct {
	ServerURL       string `json:"serverURL"`
	ListenAddr      string `json:"listenAddr"`
	DefaultBackend  string `json:"defaultBackend"`
	DefaultModel    string `json:"defaultModel"`
	SkipPermissions bool   `json:"skipPermissions"`
	PlanMode        bool   `json:"planMode"`
}

// defaultClientConfig returns built-in defaults used when no config file exists.
func defaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerURL:      "http://127.0.0.1:8080",
		ListenAddr:     "127.0.0.1:8080",
		DefaultBackend: "claude",
	}
}

// OrbitorDir returns the path to ~/.orbitor/ and ensures it exists.
func OrbitorDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".orbitor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ClientConfigPath returns the path to ~/.orbitor/config.json.
func ClientConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".orbitor", "config.json"), nil
}

// LoadMCPServers reads MCP server definitions from the backend's native config
// file and returns them as a slice suitable for the ACP session/new mcpServers
// parameter. Returns an empty slice (never nil) if no servers are configured.
//
// Claude Code:   ~/.claude.json  → { "mcpServers": { "name": { ... } } }
// Copilot CLI:   ~/.copilot/mcp-config.json → { "mcpServers": { "name": { ... } } }
//
// Both also support a project-local .mcp.json in the working directory.
func LoadMCPServers(backend, workingDir string) []any {
	home, err := os.UserHomeDir()
	if err != nil {
		return []any{}
	}

	// Determine config file paths to read (global + project-local).
	var paths []string
	switch backend {
	case "claude":
		paths = append(paths, filepath.Join(home, ".claude.json"))
	case "copilot":
		paths = append(paths, filepath.Join(home, ".copilot", "mcp-config.json"))
	}
	if workingDir != "" {
		paths = append(paths, filepath.Join(workingDir, ".mcp.json"))
	}

	// Merge servers from all config files (later files override earlier ones).
	merged := map[string]json.RawMessage{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if json.Unmarshal(data, &cfg) != nil || cfg.MCPServers == nil {
			continue
		}
		for name, server := range cfg.MCPServers {
			merged[name] = server
		}
	}

	if len(merged) == 0 {
		return []any{}
	}

	// Convert the name→config map into the ACP array format:
	// [{ "name": "...", ...server_config }]
	var servers []any
	for name, raw := range merged {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		obj["name"] = name
		servers = append(servers, obj)
	}
	if servers == nil {
		return []any{}
	}
	return servers
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
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8080"
	}
	if cfg.DefaultBackend == "" {
		cfg.DefaultBackend = "claude"
	}
	return cfg
}
