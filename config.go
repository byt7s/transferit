package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type savedConfig struct {
	PasswordMode string `json:"password_mode"`
	Password     string `json:"password,omitempty"`
	Expiry       string `json:"expiry"`
}

func defaultConfig() savedConfig {
	return savedConfig{PasswordMode: "none", Expiry: "90d"}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "transferit", "config.json"), nil
}

func loadConfig() (savedConfig, error) {
	cfg := defaultConfig()
	path, err := configPath()
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
		return defaultConfig(), fmt.Errorf("reading %s: %w", path, err)
	}
	if cfg.Expiry == "" {
		cfg.Expiry = "90d"
	}
	if cfg.PasswordMode == "" {
		cfg.PasswordMode = "none"
	}
	return cfg, nil
}

func saveConfig(cfg savedConfig) error {
	path, err := configPath()
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
	return os.WriteFile(path, data, 0600)
}

func parsePasswordChoice(v string) (mode, password string) {
	trimmed := strings.TrimSpace(v)
	switch strings.ToLower(trimmed) {
	case "", "none", "off", "no":
		return "none", ""
	case "random", "rand":
		return "random", ""
	default:
		return "fixed", trimmed
	}
}

func randomPassword(n int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func parseExpiry(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "0", "never", "none", "off", "unlimited":
		return 0, nil
	}
	mult := 86400
	num := s
	if c := s[len(s)-1]; c == 'd' || c == 'h' || c == 'm' || c == 's' {
		num = s[:len(s)-1]
		switch c {
		case 'd':
			mult = 86400
		case 'h':
			mult = 3600
		case 'm':
			mult = 60
		default:
			mult = 1
		}
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid expiry %q (examples: 1d, 7d, 30d, 90d, 12h, 0)", s)
	}
	seconds := n * mult
	if n > 0 && seconds/n != mult {
		return 0, fmt.Errorf("expiry too large")
	}
	return seconds, nil
}

func canonicalExpiry(s string) (string, error) {
	sec, err := parseExpiry(s)
	if err != nil {
		return "", err
	}
	if sec == 0 {
		return "0", nil
	}
	if sec%86400 == 0 {
		return fmt.Sprintf("%dd", sec/86400), nil
	}
	if sec%3600 == 0 {
		return fmt.Sprintf("%dh", sec/3600), nil
	}
	if sec%60 == 0 {
		return fmt.Sprintf("%dm", sec/60), nil
	}
	return fmt.Sprintf("%ds", sec), nil
}

func validateExpiry(seconds int) error {
	const max = 90 * 86400
	if seconds < 1 || seconds > max {
		return fmt.Errorf("expiry must be from 1 second up to 90 days; unlimited, 180d and 365d are refused for these uploads")
	}
	return nil
}

func expiryPhrase(seconds int) string {
	if seconds <= 0 {
		return "never"
	}
	if seconds%86400 == 0 {
		d := seconds / 86400
		if d == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", d)
	}
	if seconds%3600 == 0 {
		h := seconds / 3600
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	if seconds%60 == 0 {
		m := seconds / 60
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	if seconds == 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", seconds)
}

func (c savedConfig) passwordLabel() string {
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

func runConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	password := fs.String("password", "", "none, random, or a fixed password")
	expiry := fs.String("expiry", "", "expiry: 1d, 7d, 30d, 90d, or 12h (maximum 90 days)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit config [-password VALUE] [-expiry VALUE]\n\n")
		fmt.Fprintf(os.Stderr, "With no flags, prints the saved defaults. Password VALUE is:\n")
		fmt.Fprintf(os.Stderr, "  none      no password\n")
		fmt.Fprintf(os.Stderr, "  random    generate a new password on every upload\n")
		fmt.Fprintf(os.Stderr, "  anything  use that password on every upload\n\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit config\n")
		fmt.Fprintf(os.Stderr, "  transferit config -password none -expiry 90d\n")
		fmt.Fprintf(os.Stderr, "  transferit config -password random -expiry 7d\n")
		fmt.Fprintf(os.Stderr, "  transferit config -password 's3cret' -expiry 1d\n")
	}
	fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	passwordSet, expirySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "password":
			passwordSet = true
		case "expiry":
			expirySet = true
		}
	})
	if !passwordSet && !expirySet {
		printConfig(cfg)
		return
	}
	if passwordSet {
		cfg.PasswordMode, cfg.Password = parsePasswordChoice(*password)
	}
	if expirySet {
		canon, err := canonicalExpiry(*expiry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		seconds, err := parseExpiry(canon)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := validateExpiry(seconds); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cfg.Expiry = canon
	}
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printConfig(cfg)
}

func printConfig(cfg savedConfig) {
	path, err := configPath()
	if err != nil {
		path = "(unknown)"
	}
	sec, _ := parseExpiry(cfg.Expiry)
	fmt.Printf("Config:   %s\n", path)
	fmt.Printf("Password: %s\n", cfg.passwordLabel())
	fmt.Printf("Expiry:   %s (%s)\n", cfg.Expiry, expiryPhrase(sec))
}
