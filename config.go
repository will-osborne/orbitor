package main

import (
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

// Config holds optional developer configuration loaded from config/config.yaml
type Config struct {
	WhatsApp struct {
		DefaultRecipient string `yaml:"default_recipient"` // e.g. "+15551234567"
		DBPath           string `yaml:"db_path"`           // default: "whatsmeow.db"
	} `yaml:"whatsapp"`
	// Ollama: set url to use an existing Ollama instance instead of the
	// built-in llamafile approach (which downloads and manages its own server).
	Ollama struct {
		URL   string `yaml:"url"`   // e.g. "http://localhost:11434"
		Model string `yaml:"model"` // default: "qwen2.5:1.5b"
	} `yaml:"ollama"`
	// LLM controls the built-in llamafile summarizer (used when Ollama.URL is empty).
	LLM struct {
		CacheDir string `yaml:"cache_dir"` // default: ~/.cache/copilot-bridge
		ModelURL string `yaml:"model_url"` // override model download URL
	} `yaml:"llm"`
}

// AppConfig is the global configuration instance (may be nil)
var AppConfig *Config

// LoadConfig reads the YAML config at path and populates AppConfig.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	AppConfig = &cfg
	return &cfg, nil
}
