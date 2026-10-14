package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
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
	fmt.Fprintf(os.Stderr, "\nRun 'transferit <command> -h' for more information on a command.\n")
}

func runDownload(args []string) {
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("o", ".", "output directory")
	password := fs.String("p", "", "password for protected transfers")
	listOnly := fs.Bool("l", false, "list files without downloading")
	verbose := fs.Bool("v", false, "show detailed API info")
	silent := fs.Bool("s", false, "suppress all output")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit download [options] <transfer.it URL or handle>\n\nOptions:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit dl https://transfer.it/t/123123123\n")
		fmt.Fprintf(os.Stderr, "  transferit d 123123123 -o ~/Downloads\n")
		fmt.Fprintf(os.Stderr, "  transferit download 123123123 -l\n")
	}

	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	dl := NewDownloader(*verbose, *silent)
	if err := dl.Run(fs.Arg(0), *output, *password, *listOnly); err != nil {
		if err.Error() == "download aborted by user" {
			fmt.Println("Download aborted.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
