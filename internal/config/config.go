// Package config reads and writes the saved upload password and expiry.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	PasswordMode string `json:"password_mode"`
	Password     string `json:"password,omitempty"`
	Expiry       string `json:"expiry"`
}

func Default() Config {
	return Config{PasswordMode: "none", Expiry: "90d"}
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "transferit", "config.json"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("reading %s: %w", path, err)
	}
	if cfg.Expiry == "" {
		cfg.Expiry = "90d"
	}
	if cfg.PasswordMode == "" {
		cfg.PasswordMode = "none"
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if cfg.PasswordMode != "fixed" {
		cfg.Password = ""
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return restrictPath(path)
}

func (c Config) PasswordLabel() string {
	switch c.PasswordMode {
	case "random":
		return "random (a new password on every upload)"
	case "fixed":
		if strings.TrimSpace(c.Password) == "" {
			return "none"
		}
		return c.Password
	default:
		return "none"
	}
}
