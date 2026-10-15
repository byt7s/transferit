package main

import (
	"flag"
	"fmt"
	"os"

	"transferit/internal/download"
)

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	password := fs.String("p", "", "password for a protected transfer")
	verbose := fs.Bool("v", false, "show API requests")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit info [options] <transfer.it URL or handle>\n\n")
		fmt.Fprintf(os.Stderr, "Show the title, size, and file list without downloading.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit info https://transfer.it/t/123123123\n")
		fmt.Fprintf(os.Stderr, "  transferit info 123123123 -p secret\n")
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	if err := download.New(*verbose, false).Info(fs.Arg(0), *password); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
