package ui

import (
	"fmt"
	"os"
	"strings"

	"transferit/internal/format"
)

type File struct {
	Name string
	Size int64
}

func layoutSteps(width, step int, msg, title string, totalBytes int64, fileCount, folderCount int, files []File) []string {
	lines := appendBrand(nil, width)
	labels := []string{"Transfer", "Files", "Decrypt", "Download"}
	for i, label := range labels {
		switch {
		case i+1 < step:
			lines = append(lines, "  "+tone(ansiGreen, "✓")+"  "+tone(ansiDim, label))
		case i+1 == step:
			lines = append(lines, "  "+tone(ansiBold+ansiCyan, "▸")+"  "+tone(ansiBold, label))
		default:
			lines = append(lines, "  "+tone(ansiDim, "·")+"  "+tone(ansiDim, label))
		}
	}
	lines = append(lines, "")
	if msg != "" {
		lines = append(lines, "  "+clipRunes(msg, width))
		lines = append(lines, "")
	}
	if title != "" {
		lines = append(lines, "  "+tone(ansiBold, clipRunes(title, width)))
	}
	if totalBytes > 0 || fileCount > 0 || folderCount > 0 {
		lines = append(lines, "  "+tone(ansiDim, clipRunes(Summary(totalBytes, fileCount, folderCount), width)))
	}
	if len(files) > 0 {
		lines = append(lines, "")
		shown := files
		extra := 0
		if len(shown) > 5 {
			extra = len(shown) - 5
			shown = shown[:5]
		}
		for _, file := range shown {
			lines = append(lines, fileRow(width, file.Name, file.Size))
		}
		if extra > 0 {
			lines = append(lines, "  "+tone(ansiDim, fmt.Sprintf("and %d more", extra)))
		}
	}
	lines = append(lines, "", hintLine(width, "ctrl+c", "cancel"))
	return lines
}

func Summary(totalBytes int64, fileCount, folderCount int) string {
	parts := make([]string, 0, 3)
	if totalBytes > 0 {
		parts = append(parts, format.Size(totalBytes))
	}
	if fileCount > 0 {
		word := "files"
		if fileCount == 1 {
			word = "file"
		}
		parts = append(parts, fmt.Sprintf("%d %s", fileCount, word))
	}
	if folderCount > 0 {
		word := "folders"
		if folderCount == 1 {
			word = "folder"
		}
		parts = append(parts, fmt.Sprintf("%d %s", folderCount, word))
	}
	return strings.Join(parts, "  ·  ")
}

func fileRow(width int, name string, size int64) string {
	sizeText := format.Size(size)
	room := width - runeCount(sizeText) - 2
	if room < 8 {
		room = 8
	}
	label := clipRunes(name, room)
	gap := width - runeCount(label) - runeCount(sizeText)
	if gap < 1 {
		gap = 1
	}
	return "  " + label + strings.Repeat(" ", gap) + tone(ansiDim, sizeText)
}

func RenderStep(pretty bool, step int, msg, title string, totalBytes int64, files, folders []File) {
	if pretty {
		Draw(layoutSteps(ContentWidth(), step, msg, title, totalBytes, len(files), len(folders), files))
		return
	}
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	if title != "" {
		fmt.Fprintf(os.Stderr, "title  %s\n", title)
	}
	if len(files) > 0 || len(folders) > 0 {
		fmt.Fprintln(os.Stderr, Summary(totalBytes, len(files), len(folders)))
		limit := len(files)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", files[i].Name, format.Size(files[i].Size))
		}
		if len(files) > limit {
			fmt.Fprintf(os.Stderr, "  and %d more\n", len(files)-limit)
		}
	}
}

func PrintList(title string, totalBytes int64, files, folders []File) {
	fmt.Println()
	if title != "" {
		fmt.Printf("  %s\n", title)
	}
	fmt.Printf("  %s\n", Summary(totalBytes, len(files), len(folders)))
	if len(files) == 0 {
		return
	}
	fmt.Println()
	for _, file := range files {
		fmt.Printf("  %s (%s)\n", file.Name, format.Size(file.Size))
	}
}
