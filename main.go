package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	output := flag.String("o", ".", "output directory")
	listOnly := flag.Bool("l", false, "list files without downloading")
	verbose := flag.Bool("v", false, "show detailed API info")
	silent := flag.Bool("s", false, "suppress all output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <transfer.it URL or handle>\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s https://transfer.it/t/123123123\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s 123123123 -o ~/Downloads\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s 123123123 -l\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	dl := NewDownloader(*verbose, *silent)
	if err := dl.Run(flag.Arg(0), *output, *listOnly); err != nil {
		if err.Error() == "download aborted by user" {
			fmt.Println("Download aborted.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
