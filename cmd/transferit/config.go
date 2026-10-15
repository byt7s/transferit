package main

import (
	"flag"
	"fmt"
	"os"

	"transferit/internal/config"
)

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

	cfg, err := config.Load()
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
		cfg.PasswordMode, cfg.Password = config.ParsePassword(*password)
	}
	if expirySet {
		canon, err := config.CanonicalExpiry(*expiry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		seconds, err := config.ParseExpiry(canon)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateExpiry(seconds); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cfg.Expiry = canon
	}
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printConfig(cfg)
}

func printConfig(cfg config.Config) {
	path, err := config.Path()
	if err != nil {
		path = "(unknown)"
	}
	sec, _ := config.ParseExpiry(cfg.Expiry)
	fmt.Printf("Config:   %s\n", path)
	fmt.Printf("Password: %s\n", cfg.PasswordLabel())
	fmt.Printf("Expiry:   %s (%s)\n", cfg.Expiry, config.ExpiryPhrase(sec))
}
