package main

import (
	"fmt"
	"os"

	"transferit/internal/console"
	"transferit/internal/version"
)

func main() {
	console.Enable()
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "d", "dl", "download":
		runDownload(os.Args[2:])
	case "u", "up", "upload":
		runUpload(os.Args[2:])
	case "config", "defaults":
		runConfig(os.Args[2:])
	case "version", "--version", "-version", "-V":
		version.Print()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: transferit <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  u, up, upload      Upload files or folders to transfer.it\n")
	fmt.Fprintf(os.Stderr, "  d, dl, download    Download files from transfer.it\n")
	fmt.Fprintf(os.Stderr, "  config, defaults   Show or save default password and expiry\n")
	fmt.Fprintf(os.Stderr, "  version            Show the build version\n")
	fmt.Fprintf(os.Stderr, "\nRun 'transferit <command> -h' for more information on a command.\n")
}
