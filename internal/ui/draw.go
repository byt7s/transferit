package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiInvert = "\033[7m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

var screenOut io.Writer = os.Stdout

func tone(code, text string) string {
	if text == "" {
		return ""
	}
	if os.Getenv("NO_COLOR") != "" && code != ansiInvert {
		return text
	}
	return code + text + ansiReset
}

func ContentWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 20 {
		width = 80
	}
	width -= 4
	if width > 68 {
		width = 68
	}
	if width < 20 {
		width = 20
	}
	return width
}

func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func runeCount(s string) int {
	return utf8.RuneCountInString(s)
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func Draw(lines []string) {
	var frame strings.Builder
	frame.WriteString("\033[H")
	for _, line := range lines {
		frame.WriteString("\r")
		frame.WriteString(line)
		frame.WriteString("\033[K\r\n")
	}
	frame.WriteString("\033[J")
	fmt.Fprint(screenOut, frame.String())
}
