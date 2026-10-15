package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"transferit/internal/format"
)

func layoutProgress(width int, action, filename string, downloaded, total int64, start time.Time) []string {
	elapsed := time.Since(start).Seconds()
	speed := 0.0
	if elapsed > 0.05 {
		speed = float64(downloaded) / elapsed
	}
	lines := appendBrand(nil, width)
	lines = append(lines,
		"  "+tone(ansiBold, clipRunes(action, width)),
		"  "+tone(ansiDim, clipRunes(filename, width)),
		"",
	)
	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		if pct > 100 {
			pct = 100
		}
		filled, empty := barCells(pct, width)
		lines = append(lines, "  "+tone(ansiCyan, strings.Repeat("█", filled))+tone(ansiDim, strings.Repeat("░", empty)))
		lines = append(lines, "")
		lines = append(lines, "  "+tone(ansiBold, fmt.Sprintf("%.0f%%", pct))+"   "+format.Size(downloaded)+" of "+format.Size(total))
	} else {
		lines = append(lines, "  "+tone(ansiCyan, strings.ToLower(action)))
		lines = append(lines, "")
		lines = append(lines, "  "+tone(ansiBold, format.Size(downloaded)))
	}
	eta := "--"
	if total > downloaded && speed > 0 {
		eta = format.ETA(float64(total-downloaded) / speed)
	}
	speedText := "--"
	if speed > 0 {
		speedText = format.Speed(speed)
	}
	stats := speedText + "   ·   " + eta + " left   ·   " + format.Duration(time.Since(start))
	lines = append(lines,
		"  "+tone(ansiDim, clipRunes(stats, width)),
		"",
		hintLine(width, "ctrl+c", "cancel"),
	)
	return lines
}

func barCells(pct float64, width int) (filled, empty int) {
	if width < 1 {
		width = 1
	}
	filled = int(float64(width)*pct/100 + 0.5)
	if filled <= 0 && pct > 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return filled, width - filled
}

func printTransferProgress(name string, done, total int64, start time.Time) {
	if total <= 0 {
		total = 1
	}
	if done > total {
		done = total
	}
	elapsed := time.Since(start).Seconds()
	speed := 0.0
	if elapsed > 0.05 {
		speed = float64(done) / elapsed
	}
	pct := float64(done) / float64(total) * 100
	label := clipRunes(name, 28)
	filled, empty := barCells(pct, 16)
	if stderrIsTerminal() && os.Getenv("NO_COLOR") == "" {
		bar := tone(ansiCyan, strings.Repeat("█", filled)) + tone(ansiDim, strings.Repeat("░", empty))
		fmt.Fprintf(os.Stderr, "\r\033[K  %s  %s  %5.1f%%  %8s  %s", label, bar, pct, format.Size(done), format.Speed(speed))
		return
	}
	line := fmt.Sprintf("  %-28s %5.1f%% %8s %s", label, pct, format.Size(done), format.Speed(speed))
	if len(line) < 72 {
		line += strings.Repeat(" ", 72-len(line))
	}
	fmt.Fprintf(os.Stderr, "\r%s", line)
}

func ShowProgress(pretty bool, action, name string, done, total int64, start time.Time) {
	if pretty {
		Draw(layoutProgress(ContentWidth(), action, name, done, total, start))
		return
	}
	printTransferProgress(name, done, total, start)
}
