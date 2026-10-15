//go:build !windows

package ui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func readClipboard() string {
	var commands [][]string
	if runtime.GOOS == "darwin" {
		commands = [][]string{{"pbpaste"}}
	} else {
		commands = [][]string{
			{"wl-paste", "-n"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
	}
	for _, argv := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
		cancel()
		if err != nil {
			continue
		}
		return strings.TrimRight(string(out), "\r\n")
	}
	return ""
}
