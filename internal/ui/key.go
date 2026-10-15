package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
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
