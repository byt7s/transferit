package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"transferit/internal/config"
	"transferit/internal/format"
	"transferit/internal/ui"
	"transferit/internal/upload"
)

func runUpload(args []string) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	password := fs.String("p", "", "password override: none, random, or a password (default: saved setting)")
	expiry := fs.String("e", "", "expiry override, for example 1d, 7d, 30d, 90d, or 12h (maximum 90 days)")
	title := fs.String("t", "", "transfer title")
	verbose := fs.Bool("v", false, "show API requests")
	silent := fs.Bool("s", false, "print only the link and password")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit upload [options] <file or folder>...\n\n")
		fmt.Fprintf(os.Stderr, "Folders are zipped and uploaded as a single .zip file.\n")
		fmt.Fprintf(os.Stderr, "Password and expiry come from 'transferit config' unless overridden.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit up report.pdf\n")
		fmt.Fprintf(os.Stderr, "  transferit up -p random -e 7d ./photos\n")
		fmt.Fprintf(os.Stderr, "  transferit upload -p none -e 1d -t Notes a.txt b.txt\n")
	}
	fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	mode, pw := cfg.PasswordMode, cfg.Password
	expStr := cfg.Expiry
	passwordSet, expirySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "p":
			passwordSet = true
		case "e":
			expirySet = true
		}
	})
	if passwordSet {
		mode, pw = config.ParsePassword(*password)
	}
	if expirySet {
		expStr = *expiry
	}
	seconds, err := config.ParseExpiry(expStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := config.ValidateExpiry(seconds); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	switch mode {
	case "random":
		pw, err = config.RandomPassword(16)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "fixed":
		pw = strings.TrimSpace(pw)
	default:
		pw = ""
	}

	pretty := false
	if !*silent && ui.StdinIsTerminal() {
		if err := ui.Enter(); err == nil {
			pretty = true
		}
	}
	defer ui.Exit()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ui.SetCtrlC(func() { stop() })

	if pretty {
		ui.Draw(ui.LayoutStatus(ui.ContentWidth(), "Upload", "Preparing files"))
	} else if !*silent {
		fmt.Fprintf(os.Stderr, "Preparing files...\n")
	}
	items, err := upload.Prepare(fs.Args())
	if err != nil {
		ui.Exit()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, item := range items {
			if item.Cleanup != nil {
				item.Cleanup()
			}
		}
	}()

	transferTitle := strings.TrimSpace(*title)
	if transferTitle == "" {
		if len(items) == 1 {
			transferTitle = items[0].Title
		} else {
			transferTitle = "Transfer"
		}
	}

	up := upload.New(*verbose, *silent)
	up.Pretty = pretty
	result, err := up.Run(ctx, items, upload.Options{
		Title:    transferTitle,
		Password: pw,
		Expiry:   seconds,
	})
	ui.Exit()
	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Upload aborted.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if *silent {
		fmt.Println(result.Link)
		if result.Password != "" {
			fmt.Println(result.Password)
		}
		return
	}
	fmt.Printf("\n  uploaded  %d %s (%s)\n\n", result.Files, fileWord(result.Files), format.Size(result.Bytes))
	fmt.Printf("  link      %s\n", result.Link)
	fmt.Printf("  title     %s\n", result.Title)
	if result.Password != "" {
		fmt.Printf("  password  %s\n", result.Password)
	} else {
		fmt.Printf("  password  none\n")
	}
	fmt.Printf("  expires   %s\n", config.ExpiryPhrase(result.Expiry))
}

func fileWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
