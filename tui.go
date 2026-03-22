package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	colorReset  = "\033[0m\033[37m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

var errAborted = errors.New("download aborted by user")

type fileConflictChoice int

const (
	choiceOverwrite fileConflictChoice = iota
	choiceRenameAuto
	choiceRenameManual
	choiceAbort
)

func enterAltScreen() {
	fmt.Print("\033[?1049h\033[H\033[2J")
	fmt.Print("\033[?2004h") // Enable bracketed paste mode
}

func exitAltScreen() {
	fmt.Print("\033[?2004l") // Disable bracketed paste mode
	fmt.Print("\033[?1049l")
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func moveCursor(row, col int) {
	fmt.Printf("\033[%d;%dH", row, col)
}

func hideCursor() {
	fmt.Print("\033[?25l")
}

func showCursor() {
	fmt.Print("\033[?25h")
}

func getClipboard() string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-command", "Get-Clipboard")
	} else {
		cmd = exec.Command("pbpaste")
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

const keyPaste byte = 0xFF
const keyCopy byte = 0xFE

var pastedContent string
var ctrlCAborts = true // When false, Ctrl+C returns keyCopy instead of aborting

func readKey() ([]byte, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	reader := bufio.NewReader(os.Stdin)
	
	// Read first byte
	b, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	// Ctrl+C detection
	if b == 3 {
		if ctrlCAborts {
			term.Restore(int(os.Stdin.Fd()), oldState)
			showCursor()
			exitAltScreen()
			if currentDownloadPath != "" {
				os.Remove(currentDownloadPath)
			}
			fmt.Print("Download aborted.\r\n")
			os.Exit(0)
		}
		// In input mode, Ctrl+C means copy
		return []byte{keyCopy}, nil
	}

	// Ctrl+V (22) or Ctrl+Y (25) detection - paste from clipboard
	if b == 22 || b == 25 {
		pastedContent = getClipboard()
		return []byte{keyPaste}, nil
	}

	// ESC sequence
	if b == 27 {
		// Check if more bytes available
		if reader.Buffered() > 0 || waitForInput(reader, 50) {
			second, _ := reader.ReadByte()
			if second == '[' {
				if reader.Buffered() > 0 || waitForInput(reader, 50) {
					third, _ := reader.ReadByte()
					// Bracketed paste: ESC [ 2 0 0 ~
					if third == '2' {
						// Drain all pasted content until ESC[201~
						drainUntilEndPaste(reader)
						pastedContent = getClipboard()
						return []byte{keyPaste}, nil
					}
					// Arrow keys: ESC [ A/B/C/D
					return []byte{27, '[', third}, nil
				}
				return []byte{27, '['}, nil
			}
			return []byte{27, second}, nil
		}
		return []byte{27}, nil
	}

	return []byte{b}, nil
}

func waitForInput(reader *bufio.Reader, ms int) bool {
	// Simple check - if there's buffered data, return true
	// Otherwise wait a tiny bit and check again
	if reader.Buffered() > 0 {
		return true
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return reader.Buffered() > 0
}

func drainUntilEndPaste(reader *bufio.Reader) {
	// Read until we see \x1b[201~
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		if b == 27 {
			// Check for [201~
			next := make([]byte, 5)
			n, _ := reader.Read(next)
			if n >= 4 && string(next[:4]) == "[201" {
				// Found end sequence
				break
			}
		}
	}
}

func getPastedContent() string {
	return pastedContent
}

func printEmptyLine() {
	fmt.Println("                                                                    ")
}

func printHeader() {
	printEmptyLine()
	fmt.Printf("  %s╔══════════════════════════════════════════════════════════════╗%s\n", colorCyan, colorReset)
	fmt.Printf("  %s║%s%s                         TRANSFER.IT                          %s%s║%s\n", colorCyan, colorReset, colorBold, colorReset, colorCyan, colorReset)
	fmt.Printf("  %s╚══════════════════════════════════════════════════════════════╝%s\n", colorCyan, colorReset)
	printEmptyLine()
}

func printTitle(title string) {
	fmt.Printf("  %s%s%s%s                                                    \n", colorBold, colorBlue, title, colorReset)
	printEmptyLine()
}

func printMenuOption(text string, selected bool) {
	if selected {
		fmt.Printf("  %s%s› %-60s%s\n", colorBold, colorGreen, text, colorReset)
	} else {
		fmt.Printf("  %s  %-60s%s\n", colorDim, text, colorReset)
	}
}

func printMenuHint() {
	printEmptyLine()
	fmt.Printf("  %sUse ↑/↓ arrows to select, Enter to confirm%s                       \n", colorDim, colorReset)
}

func printInputField(prefix, value string, maxLen int) {
	display := value
	if len(display) > maxLen {
		display = display[:maxLen]
	}
	fmt.Printf("  %s>%s %s%-60s%s\n", colorGreen, colorReset, colorBold, display, colorReset)
}

func (d *Downloader) showRenameSubmenu() int {
	options := []string{
		"Auto-rename (add number suffix)",
		"Manual rename",
		"← Back",
	}
	selected := 0
	firstRender := true

	for {
		if firstRender {
			clearScreen()
			firstRender = false
		}
		moveCursor(1, 1)
		printHeader()
		printTitle("RENAME OPTIONS")

		for i, opt := range options {
			printMenuOption(opt, i == selected)
		}
		printMenuHint()

		key, err := readKey()
		if err != nil {
			continue
		}

		if len(key) == 3 && key[0] == 27 && key[1] == 91 {
			switch key[2] {
			case 65: // Up arrow
				if selected > 0 {
					selected--
				}
			case 66: // Down arrow
				if selected < len(options)-1 {
					selected++
				}
			}
		} else if len(key) == 1 && key[0] == 13 { // Enter
			return selected
		}
	}
}

func (d *Downloader) showPasswordInput() string {
	password := []rune{}
	cursor := 0
	firstRender := true

	// Disable Ctrl+C abort during input
	ctrlCAborts = false
	defer func() { ctrlCAborts = true }()

	pasteHint := "Ctrl+V"
	if runtime.GOOS == "darwin" {
		pasteHint = "Cmd+V"
	}

	for {
		if firstRender {
			clearScreen()
			firstRender = false
		}
		moveCursor(1, 1)
		printHeader()
		fmt.Printf("  %s%s🔒 PASSWORD PROTECTED%s                                             \n", colorBold, colorYellow, colorReset)
		printEmptyLine()
		fmt.Printf("  %sThis transfer requires a password to access.%s                      \n", colorWhite, colorReset)
		printEmptyLine()
		fmt.Printf("  %sEnter password:%s                                                   \n", colorWhite, colorReset)
		printEmptyLine()
		printInputField(">", string(password), 55)
		printEmptyLine()
		fmt.Printf("  %sEnter to confirm, Esc to cancel, %s to paste%s                   \n", colorDim, pasteHint, colorReset)

		// Position cursor visually
		moveCursor(12, 5+cursor)
		showCursor()

		key, err := readKey()
		if err != nil {
			continue
		}

		hideCursor()

		if len(key) == 1 {
			switch key[0] {
			case 13: // Enter
				return string(password)
			case 27: // Esc
				return ""
			case 127, 8: // Backspace
				if cursor > 0 {
					password = append(password[:cursor-1], password[cursor:]...)
					cursor--
				}
			case keyPaste: // Cmd+V (macOS) or Ctrl+V
				clip := []rune(getPastedContent())
				password = append(password[:cursor], append(clip, password[cursor:]...)...)
				cursor += len(clip)
			default:
				if key[0] >= 32 && key[0] < 127 {
					password = append(password[:cursor], append([]rune{rune(key[0])}, password[cursor:]...)...)
					cursor++
				}
			}
		} else if len(key) == 3 && key[0] == 27 && key[1] == 91 {
			switch key[2] {
			case 68: // Left arrow
				if cursor > 0 {
					cursor--
				}
			case 67: // Right arrow
				if cursor < len(password) {
					cursor++
				}
			}
		}
	}
}

func (d *Downloader) showManualRenameInput(originalName string) string {
	name := []rune(originalName)
	cursor := len(name)
	firstRender := true

	// Disable Ctrl+C abort during input
	ctrlCAborts = false
	defer func() { ctrlCAborts = true }()

	for {
		if firstRender {
			clearScreen()
			firstRender = false
		}
		moveCursor(1, 1)
		printHeader()
		printTitle("MANUAL RENAME")
		fmt.Printf("  %sEnter new filename:%s                                               \n", colorWhite, colorReset)
		printEmptyLine()
		printInputField(">", string(name), 55)
		printEmptyLine()
		fmt.Printf("  %sType to edit, Enter to confirm, Esc to cancel%s                    \n", colorDim, colorReset)

		// Position cursor visually
		moveCursor(10, 5+cursor)
		showCursor()

		key, err := readKey()
		if err != nil {
			continue
		}

		hideCursor()

		if len(key) == 1 {
			switch key[0] {
			case 13: // Enter
				if len(name) > 0 {
					return string(name)
				}
			case 27: // Esc
				return ""
			case 127, 8: // Backspace
				if cursor > 0 {
					name = append(name[:cursor-1], name[cursor:]...)
					cursor--
				}
			case keyPaste: // Cmd+V (macOS) or Ctrl+V
				clip := []rune(getPastedContent())
				name = append(name[:cursor], append(clip, name[cursor:]...)...)
				cursor += len(clip)
			default:
				if key[0] >= 32 && key[0] < 127 {
					name = append(name[:cursor], append([]rune{rune(key[0])}, name[cursor:]...)...)
					cursor++
				}
			}
		} else if len(key) == 3 && key[0] == 27 && key[1] == 91 {
			switch key[2] {
			case 68: // Left arrow
				if cursor > 0 {
					cursor--
				}
			case 67: // Right arrow
				if cursor < len(name) {
					cursor++
				}
			}
		}
	}
}

func (d *Downloader) showFileConflictMenu(existingPath string, fileInfo os.FileInfo, originalFilename string) (fileConflictChoice, string) {
	options := []string{
		"Overwrite existing file",
		"Rename download file",
		"Abort download",
	}
	selected := 0
	firstRender := true

	modTime := fileInfo.ModTime()
	size := fileInfo.Size()

	for {
		if firstRender {
			clearScreen()
			firstRender = false
		}
		moveCursor(1, 1)
		printHeader()
		fmt.Printf("  %s%s⚠  FILE ALREADY EXISTS%s                                           \n", colorBold, colorYellow, colorReset)
		printEmptyLine()
		fmt.Printf("  %sPath:%s     %s%-52s%s\n", colorDim, colorReset, colorWhite, existingPath, colorReset)
		fmt.Printf("  %sSize:%s     %s%-52s%s\n", colorDim, colorReset, colorWhite, formatSize(size), colorReset)
		fmt.Printf("  %sModified:%s %s%-52s%s\n", colorDim, colorReset, colorWhite, modTime.Format("2006/01/02 15:04"), colorReset)
		printEmptyLine()
		fmt.Printf("  %sChoose an action:%s                                                 \n", colorWhite, colorReset)
		printEmptyLine()

		for i, opt := range options {
			printMenuOption(opt, i == selected)
		}
		printMenuHint()

		key, err := readKey()
		if err != nil {
			continue
		}

		if len(key) == 3 && key[0] == 27 && key[1] == 91 {
			switch key[2] {
			case 65: // Up arrow
				if selected > 0 {
					selected--
				}
			case 66: // Down arrow
				if selected < len(options)-1 {
					selected++
				}
			}
		} else if len(key) == 1 && key[0] == 13 { // Enter
			if selected == 0 {
				return choiceOverwrite, ""
			} else if selected == 1 {
				// Show rename submenu
				subChoice := d.showRenameSubmenu()
				if subChoice == 0 {
					return choiceRenameAuto, ""
				} else if subChoice == 1 {
					newName := d.showManualRenameInput(originalFilename)
					if newName != "" {
						return choiceRenameManual, newName
					}
					// User cancelled, go back to main menu
					continue
				}
				// Back option, continue main menu
				continue
			} else {
				return choiceAbort, ""
			}
		}
	}
}

func (d *Downloader) renderTUI(filename string, downloaded, total int64, startTime time.Time) {
	elapsed := time.Since(startTime).Seconds()
	speed := float64(downloaded) / elapsed
	remaining := total - downloaded
	var eta string
	if speed > 0 {
		etaSecs := float64(remaining) / speed
		eta = formatETA(etaSecs)
	} else {
		eta = "--"
	}
	pct := float64(downloaded) / float64(total) * 100
	barLen := 50
	filled := int(float64(barLen) * float64(downloaded) / float64(total))
	barFilled := strings.Repeat("█", filled)
	barEmpty := strings.Repeat("░", barLen-filled)

	moveCursor(1, 1)
	printHeader()
	printEmptyLine()
	fmt.Printf("  %sFile:%s       %s%-58s%s\n", colorDim, colorReset, colorBold, filename, colorReset)
	fmt.Printf("  %sElapsed:%s    %s%-50s%s\n", colorDim, colorReset, colorBold, formatDuration(time.Since(startTime)), colorReset)
	fmt.Printf("  %sDownloaded:%s %s%-50s%s\n", colorDim, colorReset, colorBold, fmt.Sprintf("%s / %s", formatSize(downloaded), formatSize(total)), colorReset)
	printEmptyLine()
	fmt.Printf("  [%s%s%s%s%s] %s%5.1f%%%s\n", colorGreen, barFilled, colorReset, colorDim, barEmpty+colorReset, colorBold, pct, colorReset)
	printEmptyLine()
	fmt.Printf("  %sETA:%s        %s%-50s%s\n", colorDim, colorReset, colorBold, eta, colorReset)
	fmt.Printf("  %sSpeed:%s      %s%-50s%s\n", colorDim, colorReset, colorBold, formatSpeed(speed), colorReset)
}

func (d *Downloader) renderStep(step int, msg string, info *TransferInfo, title string, files []FileNode) {
	clearScreen()
	moveCursor(1, 1)
	printHeader()

	steps := []string{
		"Fetching transfer info",
		"Fetching file listing",
		"Decrypting file attributes",
		"Downloading",
	}

	for i, s := range steps {
		if i+1 < step {
			fmt.Printf("  %s%s✓%s Step %d: %s%s%s\n", colorBold, colorGreen, colorReset, i+1, colorGreen, s, colorReset)
		} else if i+1 == step {
			fmt.Printf("  %s%s►%s Step %d: %s%s%s\n", colorBold, colorYellow, colorReset, i+1, colorBold, s, colorReset)
		} else {
			fmt.Printf("  %s  Step %d: %s%s\n", colorDim, i+1, s, colorReset)
		}
	}

	fmt.Println()
	if msg != "" {
		fmt.Printf("  %s%s%s\n", colorCyan, msg, colorReset)
	}

	if info != nil && step >= 2 {
		fmt.Println()
		if title != "" {
			fmt.Printf("  %sTitle:%s %s%s%s\n", colorDim, colorReset, colorBold, title, colorReset)
		}
		var totalBytes int64
		if len(info.Size) > 0 {
			totalBytes = info.Size[0]
		}
		fmt.Printf("  %sSize:%s  %s%s%s\n", colorDim, colorReset, colorWhite, formatSize(totalBytes), colorReset)
	}

	if len(files) > 0 && step >= 3 {
		fmt.Println()
		fmt.Printf("  %s %d files found %s\n", colorDim, len(files), colorReset)
		maxShow := 5
		if len(files) < maxShow {
			maxShow = len(files)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Printf("    %s•%s %s (%s%s%s)\n", colorGreen, colorReset, files[i].Name, colorCyan, formatSize(files[i].S), colorReset)
		}
		if len(files) > 5 {
			fmt.Printf("    %s... and %d more%s\n", colorDim, len(files)-5, colorReset)
		}
	}
}
