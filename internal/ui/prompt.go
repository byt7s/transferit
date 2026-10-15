package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"transferit/internal/format"
)

type ConflictChoice int

const (
	Overwrite ConflictChoice = iota
	RenameAuto
	RenameManual
	Abort
)

func AskPassword(pretty bool, warning string) (string, error) {
	if pretty {
		return showPasswordInput(warning), nil
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

func AskConflict(pretty bool, existingPath string, fileInfo os.FileInfo, originalFilename string) (ConflictChoice, string, error) {
	if pretty {
		choice, name := showFileConflictMenu(existingPath, fileInfo, originalFilename)
		return choice, name, nil
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "already saved  %s\n", filepathBase(existingPath))
	fmt.Fprintf(os.Stderr, "  %s · %s\n", format.Size(fileInfo.Size()), fileInfo.ModTime().Format("2 Jan 2006 15:04"))
	options := []string{"Overwrite", "Keep both", "Rename", "Cancel"}
	for i, option := range options {
		fmt.Fprintf(os.Stderr, "  %d  %s\n", i+1, option)
	}
	for {
		line, err := readPromptLine("choice: ")
		if err != nil {
			return Abort, "", err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintln(os.Stderr, "enter a number from the list")
			continue
		}
		switch n {
		case 1:
			return Overwrite, "", nil
		case 2:
			return RenameAuto, "", nil
		case 3:
			name, err := readPromptLine("new filename: ")
			if err != nil {
				return Abort, "", err
			}
			if strings.TrimSpace(name) == "" {
				return Abort, "", nil
			}
			return RenameManual, name, nil
		default:
			return Abort, "", nil
		}
	}
}

func showPasswordInput(warning string) string {
	value := []rune{}
	cursor := 0
	for {
		Draw(layoutPassword(ContentWidth(), warning, value, cursor))
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

func showManualRenameInput(originalName string) string {
	value := []rune(originalName)
	cursor := len(value)
	for {
		Draw(layoutRename(ContentWidth(), value, cursor))
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

func showFileConflictMenu(existingPath string, fileInfo os.FileInfo, originalFilename string) (ConflictChoice, string) {
	options := []string{"Overwrite", "Keep both", "Rename", "Cancel"}
	selected := 0
	meta := []string{
		format.Size(fileInfo.Size()) + "  ·  " + fileInfo.ModTime().Format("2 Jan 2006 · 15:04"),
		filepathDir(existingPath),
	}
	for {
		Draw(layoutMenu(ContentWidth(), "Already saved", filepathBase(existingPath), meta, options, selected))
		key, err := readKey()
		if err != nil {
			return Abort, ""
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
			return Abort, ""
		case keyRune:
			if key.r >= '1' && int(key.r-'1') < len(options) {
				selected = int(key.r - '1')
				choice, name := finishConflict(selected, originalFilename)
				if choice == RenameManual && name == "" {
					continue
				}
				return choice, name
			}
		case keyEnter:
			choice, name := finishConflict(selected, originalFilename)
			if choice == RenameManual && name == "" {
				continue
			}
			return choice, name
		}
	}
}

func finishConflict(selected int, originalFilename string) (ConflictChoice, string) {
	switch selected {
	case 0:
		return Overwrite, ""
	case 1:
		return RenameAuto, ""
	case 2:
		name := showManualRenameInput(originalFilename)
		if name == "" {
			return RenameManual, ""
		}
		return RenameManual, name
	default:
		return Abort, ""
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
