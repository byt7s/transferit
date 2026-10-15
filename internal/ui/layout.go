package ui

import (
	"runtime"
	"strings"
)

func appendBrand(lines []string, width int) []string {
	lines = append(lines,
		"",
		"  "+tone(ansiBold+ansiCyan, "transfer.it"),
		"  "+tone(ansiDim, strings.Repeat("─", width)),
		"",
	)
	return lines
}

func pasteChord() string {
	if runtime.GOOS == "darwin" {
		return "cmd+v"
	}
	return "ctrl+v"
}

func hintLine(width int, pairs ...string) string {
	if len(pairs)%2 != 0 {
		pairs = pairs[:len(pairs)-1]
	}
	for len(pairs) >= 2 {
		parts := make([]string, 0, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			parts = append(parts, tone(ansiBold, pairs[i])+" "+tone(ansiDim, pairs[i+1]))
		}
		line := "  " + strings.Join(parts, "   ")
		if runeCount(stripANSI(line)) <= width+2 {
			return line
		}
		pairs = pairs[:len(pairs)-2]
	}
	return ""
}

func windowRunes(value []rune, cursor, width int) ([]rune, int) {
	if width < 1 {
		width = 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	if len(value) <= width {
		return value, cursor
	}
	start := cursor - width + 1
	if start < 0 {
		start = 0
	}
	if start > len(value)-width {
		start = len(value) - width
	}
	return value[start : start+width], cursor - start
}

func inputBox(width int, value []rune, cursor int) []string {
	inner := width - 2
	if inner < 4 {
		inner = 4
	}
	textWidth := inner - 2
	view, viewCursor := windowRunes(value, cursor, textWidth-1)
	var body strings.Builder
	body.WriteString("  ")
	body.WriteString(tone(ansiCyan, "│ "))
	visible := 0
	if len(view) == 0 {
		body.WriteString(tone(ansiInvert, " "))
		visible = 1
	} else {
		for i, r := range view {
			ch := string(r)
			if i == viewCursor {
				ch = tone(ansiInvert, ch)
			}
			body.WriteString(ch)
			visible++
		}
		if viewCursor == len(view) {
			body.WriteString(tone(ansiInvert, " "))
			visible++
		}
	}
	if pad := textWidth - visible; pad > 0 {
		body.WriteString(strings.Repeat(" ", pad))
	}
	body.WriteString(tone(ansiCyan, " │"))
	rule := strings.Repeat("─", inner)
	return []string{
		"  " + tone(ansiCyan, "┌"+rule+"┐"),
		body.String(),
		"  " + tone(ansiCyan, "└"+rule+"┘"),
	}
}

func layoutPassword(width int, warning string, value []rune, cursor int) []string {
	lines := appendBrand(nil, width)
	lines = append(lines,
		"  "+tone(ansiBold, "Password"),
		"  "+tone(ansiDim, "This transfer is protected."),
		"",
	)
	if warning != "" {
		lines = append(lines, "  "+tone(ansiYellow, clipRunes(warning, width)), "")
	}
	lines = append(lines, inputBox(width, value, cursor)...)
	lines = append(lines, "", hintLine(width, "enter", "confirm", "esc", "cancel", pasteChord(), "paste", "ctrl+c", "stop"))
	return lines
}

func layoutRename(width int, value []rune, cursor int) []string {
	lines := appendBrand(nil, width)
	lines = append(lines,
		"  "+tone(ansiBold, "Rename"),
		"  "+tone(ansiDim, "Choose the name to save."),
		"",
	)
	lines = append(lines, inputBox(width, value, cursor)...)
	lines = append(lines, "", hintLine(width, "enter", "save", "esc", "back", pasteChord(), "paste"))
	return lines
}

func layoutMenu(width int, title, subtitle string, meta []string, options []string, selected int) []string {
	lines := appendBrand(nil, width)
	lines = append(lines, "  "+tone(ansiBold, title))
	if subtitle != "" {
		lines = append(lines, "  "+tone(ansiDim, clipRunes(subtitle, width)))
	}
	for _, line := range meta {
		lines = append(lines, "  "+tone(ansiDim, clipRunes(line, width)))
	}
	lines = append(lines, "")
	for i, option := range options {
		label := clipRunes(option, width-4)
		if i == selected {
			lines = append(lines, "  "+tone(ansiBold+ansiGreen, "▸")+"  "+tone(ansiBold, label))
			continue
		}
		lines = append(lines, "     "+tone(ansiDim, label))
	}
	lines = append(lines, "", hintLine(width, "↑↓", "move", "enter", "select", "esc", "cancel"))
	return lines
}

func LayoutStatus(width int, title, detail string) []string {
	lines := appendBrand(nil, width)
	lines = append(lines,
		"  "+tone(ansiBold, clipRunes(title, width)),
		"  "+tone(ansiDim, clipRunes(detail, width)),
		"",
		hintLine(width, "ctrl+c", "cancel"),
	)
	return lines
}
