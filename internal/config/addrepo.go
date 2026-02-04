package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AddRepo adds a repository to the configuration file at cfgPath.
// If the file doesn't exist, a new config is created.
// If the name already exists, it is overwritten.
// The repo is validated before writing.
func AddRepo(cfgPath string, name string, repo RepoConfig) error {
	cfg, err := Load(cfgPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = &Config{
			Repos: make(map[string]RepoConfig),
		}
	}

	cfg.Repos[name] = repo

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
