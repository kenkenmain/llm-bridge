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
	cfg, err := loadRaw(cfgPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = &Config{
			Repos:    make(map[string]RepoConfig),
			Defaults: NewDefaults(),
		}
	}

	if cfg.Repos == nil {
		cfg.Repos = make(map[string]RepoConfig)
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

// RemoveRepo removes a repository from the configuration file at cfgPath.
// Returns an error if the config file doesn't exist or if the repo doesn't exist.
// The function does NOT delete any files on disk - it only removes the config entry.
func RemoveRepo(cfgPath, name string) error {
	cfg, err := loadRaw(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Repos == nil {
		return fmt.Errorf("repo %q not found", name)
	}

	if _, ok := cfg.Repos[name]; !ok {
		return fmt.Errorf("repo %q not found", name)
	}

	delete(cfg.Repos, name)

	// No validation required - empty repos map is valid
	// (user can still add repos later via add-repo command)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
