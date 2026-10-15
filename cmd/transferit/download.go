package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"transferit/internal/download"
)

func runDownload(args []string) {
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("o", ".", "output directory")
	password := fs.String("p", "", "password for protected transfers")
	verbose := fs.Bool("v", false, "show detailed API info")
	silent := fs.Bool("s", false, "suppress all output")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit download [options] <transfer.it URL or handle>\n\nOptions:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit dl https://transfer.it/t/123123123\n")
		fmt.Fprintf(os.Stderr, "  transferit d 123123123 -o ~/Downloads\n")
		fmt.Fprintf(os.Stderr, "  transferit info 123123123\n")
	}

	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	dl := download.New(*verbose, *silent)
	if err := dl.Run(fs.Arg(0), *output, *password); err != nil {
		if errors.Is(err, download.ErrAborted) {
			fmt.Println("Download aborted.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
