package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

var errAborted = fmt.Errorf("download aborted by user")

type fileConflictChoice int

const (
	choiceOverwrite fileConflictChoice = iota
	choiceRenameAuto
	choiceRenameManual
	choiceAbort
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

type keyKind int

const (
	keyNone keyKind = iota
	keyRune
	keyEnter
	keyEsc
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyBackspace
	keyDelete
	keyPaste
)

type keyEvent struct {
	kind keyKind
	r    rune
	text string
}

type termSession struct {
	raw    *term.State
	mu     sync.Mutex
	buf    []byte
	notify chan struct{}
}

var (
	termMu      sync.Mutex
	activeTerm  *termSession
	exitProcess = os.Exit
)

func tone(code, text string) string {
	if text == "" {
		return ""
	}
	if os.Getenv("NO_COLOR") != "" && code != ansiInvert {
		return text
	}
	return code + text + ansiReset
}

func stdinIsTerminal() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stderrIsTerminal() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func enterUI() error {
	fd := int(os.Stdin.Fd())
	if !stdinIsTerminal() {
		return fmt.Errorf("no terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	session := &termSession{
		raw:    state,
		notify: make(chan struct{}, 1),
	}
	termMu.Lock()
	activeTerm = session
	termMu.Unlock()
	fmt.Print("\033[?1049h\033[H\033[2J\033[?2004h\033[?25l")
	go session.readLoop()
	return nil
}

func exitUI() {
	termMu.Lock()
	session := activeTerm
	activeTerm = nil
	termMu.Unlock()
	if session == nil {
		return
	}
	fmt.Print("\033[?25h\033[?2004l\033[?1049l")
	_ = term.Restore(int(os.Stdin.Fd()), session.raw)
}

func abortDownload() {
	exitUI()
	discardPartialDownload()
	fmt.Fprint(os.Stdout, "Download aborted.\r\n")
	exitProcess(0)
}

func (s *termSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			for _, b := range chunk {
				if b == 3 {
					abortDownload()
				}
			}
			s.mu.Lock()
			s.buf = append(s.buf, chunk...)
			s.mu.Unlock()
			select {
			case s.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *termSession) feed(p []byte) {
	s.mu.Lock()
	s.buf = append(s.buf, p...)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *termSession) tryPop() (byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return 0, false
	}
	b := s.buf[0]
	s.buf = s.buf[1:]
	if len(s.buf) == 0 {
		s.buf = s.buf[:0]
	}
	return b, true
}

func (s *termSession) readByte(timeout time.Duration) (byte, bool) {
	if b, ok := s.tryPop(); ok {
		return b, true
	}
	if timeout == 0 {
		return 0, false
	}
	var timer <-chan time.Time
	if timeout > 0 {
		clock := time.NewTimer(timeout)
		defer clock.Stop()
		timer = clock.C
	}
	for {
		select {
		case <-s.notify:
			if b, ok := s.tryPop(); ok {
				return b, true
			}
		case <-timer:
			if b, ok := s.tryPop(); ok {
				return b, true
			}
			return 0, false
		}
	}
}

func readKey() (keyEvent, error) {
	if activeTerm == nil {
		return keyEvent{}, fmt.Errorf("terminal closed")
	}
	b, ok := activeTerm.readByte(-1)
	if !ok {
		return keyEvent{}, fmt.Errorf("terminal closed")
	}
	switch b {
	case 13:
		if n, ok := activeTerm.readByte(20 * time.Millisecond); ok && n != 10 {
			activeTerm.unread(n)
		}
		return keyEvent{kind: keyEnter}, nil
	case 10:
		return keyEvent{kind: keyEnter}, nil
	case 127, 8:
		return keyEvent{kind: keyBackspace}, nil
	case 22, 25:
		return keyEvent{kind: keyPaste, text: cleanPastedText(readClipboard())}, nil
	case 27:
		return readEscape()
	}
	if b < 32 {
		return keyEvent{kind: keyNone}, nil
	}
	r, err := decodeRune(b)
	if err != nil {
		return keyEvent{}, err
	}
	return keyEvent{kind: keyRune, r: r}, nil
}

func (s *termSession) unread(b byte) {
	s.mu.Lock()
	s.buf = append([]byte{b}, s.buf...)
	s.mu.Unlock()
}

func readEscape() (keyEvent, error) {
	b, ok := activeTerm.readByte(80 * time.Millisecond)
	if !ok {
		return keyEvent{kind: keyEsc}, nil
	}
	if b == 'O' {
		n, ok := activeTerm.readByte(80 * time.Millisecond)
		if !ok {
			return keyEvent{kind: keyEsc}, nil
		}
		switch n {
		case 'A':
			return keyEvent{kind: keyUp}, nil
		case 'B':
			return keyEvent{kind: keyDown}, nil
		case 'C':
			return keyEvent{kind: keyRight}, nil
		case 'D':
			return keyEvent{kind: keyLeft}, nil
		case 'H':
			return keyEvent{kind: keyHome}, nil
		case 'F':
			return keyEvent{kind: keyEnd}, nil
		default:
			activeTerm.unread(n)
			return keyEvent{kind: keyEsc}, nil
		}
	}
	if b != '[' {
		activeTerm.unread(b)
		return keyEvent{kind: keyEsc}, nil
	}

	var seq []byte
	for len(seq) < 32 {
		n, ok := activeTerm.readByte(80 * time.Millisecond)
		if !ok {
			break
		}
		seq = append(seq, n)
		if n >= 0x40 && n <= 0x7e {
			break
		}
	}
	body := string(seq)
	switch {
	case body == "200~":
		return keyEvent{kind: keyPaste, text: readBracketedPaste()}, nil
	case body == "3~":
		return keyEvent{kind: keyDelete}, nil
	case body == "1~" || body == "H" || body == "7~":
		return keyEvent{kind: keyHome}, nil
	case body == "4~" || body == "F" || body == "8~":
		return keyEvent{kind: keyEnd}, nil
	case strings.HasSuffix(body, "A"):
		return keyEvent{kind: keyUp}, nil
	case strings.HasSuffix(body, "B"):
		return keyEvent{kind: keyDown}, nil
	case strings.HasSuffix(body, "C"):
		return keyEvent{kind: keyRight}, nil
	case strings.HasSuffix(body, "D"):
		return keyEvent{kind: keyLeft}, nil
	default:
		return keyEvent{kind: keyNone}, nil
	}
}

func readBracketedPaste() string {
	end := []byte{27, '[', '2', '0', '1', '~'}
	var out []byte
	var match []byte
	for len(out) < 1<<20 {
		b, ok := activeTerm.readByte(2 * time.Second)
		if !ok {
			break
		}
		if b == end[len(match)] {
			match = append(match, b)
			if len(match) == len(end) {
				break
			}
			continue
		}
		if len(match) > 0 {
			out = append(out, match...)
			match = nil
			if b == end[0] {
				match = []byte{b}
				continue
			}
		}
		out = append(out, b)
	}
	return cleanPastedText(string(out))
}

func cleanPastedText(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func decodeRune(first byte) (rune, error) {
	if first < utf8.RuneSelf {
		return rune(first), nil
	}
	need := 0
	switch {
	case first&0xE0 == 0xC0:
		need = 1
	case first&0xF0 == 0xE0:
		need = 2
	case first&0xF8 == 0xF0:
		need = 3
	default:
		return utf8.RuneError, nil
	}
	buf := []byte{first}
	for i := 0; i < need; i++ {
		b, ok := activeTerm.readByte(30 * time.Millisecond)
		if !ok {
			return utf8.RuneError, nil
		}
		buf = append(buf, b)
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

func contentWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 80
	}
	width -= 4
	if width > 68 {
		width = 68
	}
	if width < 40 {
		width = 40
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

func draw(lines []string) {
	fmt.Print("\033[H")
	for _, line := range lines {
		fmt.Printf("%s\033[K\n", line)
	}
	fmt.Print("\033[J")
}

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

func layoutSteps(width, step int, msg, title string, totalBytes int64, fileCount, folderCount int, files []FileNode) []string {
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
		lines = append(lines, "  "+tone(ansiDim, clipRunes(summaryLine(totalBytes, fileCount, folderCount), width)))
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
			lines = append(lines, fileRow(width, file.Name, file.S))
		}
		if extra > 0 {
			lines = append(lines, "  "+tone(ansiDim, fmt.Sprintf("and %d more", extra)))
		}
	}
	lines = append(lines, "", hintLine(width, "ctrl+c", "cancel"))
	return lines
}

func summaryLine(totalBytes int64, fileCount, folderCount int) string {
	parts := make([]string, 0, 3)
	if totalBytes > 0 {
		parts = append(parts, formatSize(totalBytes))
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
	sizeText := formatSize(size)
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

func layoutProgress(width int, filename string, downloaded, total int64, start time.Time) []string {
	elapsed := time.Since(start).Seconds()
	speed := 0.0
	if elapsed > 0.05 {
		speed = float64(downloaded) / elapsed
	}
	lines := appendBrand(nil, width)
	lines = append(lines, "  "+tone(ansiBold, clipRunes(filename, width)), "")
	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		if pct > 100 {
			pct = 100
		}
		filled, empty := barCells(pct, width)
		lines = append(lines, "  "+tone(ansiCyan, strings.Repeat("█", filled))+tone(ansiDim, strings.Repeat("░", empty)))
		lines = append(lines, "")
		lines = append(lines, "  "+tone(ansiBold, fmt.Sprintf("%.0f%%", pct))+"   "+formatSize(downloaded)+" of "+formatSize(total))
	} else {
		lines = append(lines, "  "+tone(ansiCyan, "receiving"))
		lines = append(lines, "")
		lines = append(lines, "  "+tone(ansiBold, formatSize(downloaded)))
	}
	eta := "--"
	if total > downloaded && speed > 0 {
		eta = formatETA(float64(total-downloaded) / speed)
	}
	speedText := "--"
	if speed > 0 {
		speedText = formatSpeed(speed)
	}
	stats := speedText + "   ·   " + eta + " left   ·   " + formatDuration(time.Since(start))
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
		fmt.Fprintf(os.Stderr, "\r\033[K  %s  %s  %5.1f%%  %8s  %s", label, bar, pct, formatSize(done), formatSpeed(speed))
		return
	}
	line := fmt.Sprintf("  %-28s %5.1f%% %8s %s", label, pct, formatSize(done), formatSpeed(speed))
	if len(line) < 72 {
		line += strings.Repeat(" ", 72-len(line))
	}
	fmt.Fprintf(os.Stderr, "\r%s", line)
}

func (d *Downloader) showPasswordInput(warning string) string {
	value := []rune{}
	cursor := 0
	for {
		draw(layoutPassword(contentWidth(), warning, value, cursor))
		key, err := readKey()
		if err != nil {
			return ""
		}
		switch key.kind {
		case keyEnter:
			return string(value)
		case keyEsc:
			return ""
		case keyBackspace:
			if cursor > 0 {
				value = append(value[:cursor-1], value[cursor:]...)
				cursor--
			}
		case keyDelete:
			if cursor < len(value) {
				value = append(value[:cursor], value[cursor+1:]...)
			}
		case keyLeft:
			if cursor > 0 {
				cursor--
			}
		case keyRight:
			if cursor < len(value) {
				cursor++
			}
		case keyHome:
			cursor = 0
		case keyEnd:
			cursor = len(value)
		case keyPaste:
			value, cursor = insertRunes(value, cursor, []rune(key.text))
		case keyRune:
			value, cursor = insertRunes(value, cursor, []rune{key.r})
		}
	}
}

func (d *Downloader) showManualRenameInput(originalName string) string {
	value := []rune(originalName)
	cursor := len(value)
	for {
		draw(layoutRename(contentWidth(), value, cursor))
		key, err := readKey()
		if err != nil {
			return ""
		}
		switch key.kind {
		case keyEnter:
			if len(value) == 0 {
				continue
			}
			return string(value)
		case keyEsc:
			return ""
		case keyBackspace:
			if cursor > 0 {
				value = append(value[:cursor-1], value[cursor:]...)
				cursor--
			}
		case keyDelete:
			if cursor < len(value) {
				value = append(value[:cursor], value[cursor+1:]...)
			}
		case keyLeft:
			if cursor > 0 {
				cursor--
			}
		case keyRight:
			if cursor < len(value) {
				cursor++
			}
		case keyHome:
			cursor = 0
		case keyEnd:
			cursor = len(value)
		case keyPaste:
			value, cursor = insertRunes(value, cursor, []rune(key.text))
		case keyRune:
			value, cursor = insertRunes(value, cursor, []rune{key.r})
		}
	}
}

func insertRunes(value []rune, cursor int, extra []rune) ([]rune, int) {
	if len(extra) == 0 {
		return value, cursor
	}
	value = append(value[:cursor], append(extra, value[cursor:]...)...)
	return value, cursor + len(extra)
}

func (d *Downloader) showFileConflictMenu(existingPath string, fileInfo os.FileInfo, originalFilename string) (fileConflictChoice, string) {
	options := []string{"Overwrite", "Keep both", "Rename", "Cancel"}
	selected := 0
	meta := []string{
		formatSize(fileInfo.Size()) + "  ·  " + fileInfo.ModTime().Format("2 Jan 2006 · 15:04"),
		filepathDir(existingPath),
	}
	for {
		draw(layoutMenu(contentWidth(), "Already saved", filepathBase(existingPath), meta, options, selected))
		key, err := readKey()
		if err != nil {
			return choiceAbort, ""
		}
		switch key.kind {
		case keyUp:
			if selected > 0 {
				selected--
			}
		case keyDown:
			if selected < len(options)-1 {
				selected++
			}
		case keyEsc:
			return choiceAbort, ""
		case keyRune:
			if key.r >= '1' && int(key.r-'1') < len(options) {
				selected = int(key.r - '1')
				choice, name := d.finishConflict(selected, originalFilename)
				if choice == choiceRenameManual && name == "" {
					continue
				}
				return choice, name
			}
		case keyEnter:
			choice, name := d.finishConflict(selected, originalFilename)
			if choice == choiceRenameManual && name == "" {
				continue
			}
			return choice, name
		}
	}
}

func (d *Downloader) finishConflict(selected int, originalFilename string) (fileConflictChoice, string) {
	switch selected {
	case 0:
		return choiceOverwrite, ""
	case 1:
		return choiceRenameAuto, ""
	case 2:
		name := d.showManualRenameInput(originalFilename)
		if name == "" {
			return choiceRenameManual, ""
		}
		return choiceRenameManual, name
	default:
		return choiceAbort, ""
	}
}

func filepathBase(path string) string {
	name := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			name = path[i+1:]
			break
		}
	}
	if name == "" {
		return path
	}
	return name
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return path[:1]
			}
			return path[:i]
		}
	}
	return ""
}

func (d *Downloader) renderTUI(filename string, downloaded, total int64, startTime time.Time) {
	draw(layoutProgress(contentWidth(), filename, downloaded, total, startTime))
}

func (d *Downloader) renderStep(step int, msg string, info *TransferInfo, title string, files, folders []FileNode) {
	if d.pretty {
		var totalBytes int64
		if info != nil && len(info.Size) > 0 {
			totalBytes = info.Size[0]
		}
		draw(layoutSteps(contentWidth(), step, msg, title, totalBytes, len(files), len(folders), files))
		return
	}
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	if title != "" {
		fmt.Fprintf(os.Stderr, "title  %s\n", title)
	}
	if len(files) > 0 || len(folders) > 0 {
		var totalBytes int64
		if info != nil && len(info.Size) > 0 {
			totalBytes = info.Size[0]
		}
		fmt.Fprintln(os.Stderr, summaryLine(totalBytes, len(files), len(folders)))
		limit := len(files)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", files[i].Name, formatSize(files[i].S))
		}
		if len(files) > limit {
			fmt.Fprintf(os.Stderr, "  and %d more\n", len(files)-limit)
		}
	}
}

func (d *Downloader) askPassword(warning string) (string, error) {
	if d.pretty {
		return d.showPasswordInput(warning), nil
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	fmt.Fprint(os.Stderr, "Password: ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		buf, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}
	return readPromptLine("")
}

func (d *Downloader) askConflict(existingPath string, fileInfo os.FileInfo, originalFilename string) (fileConflictChoice, string, error) {
	if d.pretty {
		choice, name := d.showFileConflictMenu(existingPath, fileInfo, originalFilename)
		return choice, name, nil
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "already saved  %s\n", filepathBase(existingPath))
	fmt.Fprintf(os.Stderr, "  %s · %s\n", formatSize(fileInfo.Size()), fileInfo.ModTime().Format("2 Jan 2006 15:04"))
	options := []string{"Overwrite", "Keep both", "Rename", "Cancel"}
	for i, option := range options {
		fmt.Fprintf(os.Stderr, "  %d  %s\n", i+1, option)
	}
	for {
		line, err := readPromptLine("choice: ")
		if err != nil {
			return choiceAbort, "", err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintln(os.Stderr, "enter a number from the list")
			continue
		}
		switch n {
		case 1:
			return choiceOverwrite, "", nil
		case 2:
			return choiceRenameAuto, "", nil
		case 3:
			name, err := readPromptLine("new filename: ")
			if err != nil {
				return choiceAbort, "", err
			}
			if strings.TrimSpace(name) == "" {
				return choiceAbort, "", nil
			}
			return choiceRenameManual, name, nil
		default:
			return choiceAbort, "", nil
		}
	}
}

func readPromptLine(prompt string) (string, error) {
	if prompt != "" {
		fmt.Fprint(os.Stderr, prompt)
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func printSaved(results []*downloadResult) {
	if len(results) == 0 {
		return
	}
	fmt.Println()
	for _, result := range results {
		fmt.Printf("  saved  %s\n", result.path)
		fmt.Printf("         %s · %s\n", formatSize(result.size), formatDuration(result.duration))
	}
}

func printPlainList(title string, totalBytes int64, files, folders []FileNode) {
	fmt.Println()
	if title != "" {
		fmt.Printf("  %s\n", title)
	}
	fmt.Printf("  %s\n", summaryLine(totalBytes, len(files), len(folders)))
	if len(files) == 0 {
		return
	}
	fmt.Println()
	for _, file := range files {
		fmt.Printf("  %s (%s)\n", file.Name, formatSize(file.S))
	}
}
