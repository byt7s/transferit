package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestKeysStayBuffered(t *testing.T) {
	prev := activeTerm
	t.Cleanup(func() { activeTerm = prev })
	activeTerm = &termSession{notify: make(chan struct{}, 1)}
	activeTerm.feed([]byte("hi"))

	first, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	if first.kind != keyRune || first.r != 'h' || second.kind != keyRune || second.r != 'i' {
		t.Fatalf("keys = %#v %#v", first, second)
	}
}

func TestArrowAndFollowingRune(t *testing.T) {
	prev := activeTerm
	t.Cleanup(func() { activeTerm = prev })
	activeTerm = &termSession{notify: make(chan struct{}, 1)}
	activeTerm.feed([]byte{27, '[', 'A', 'x'})

	arrow, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	next, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	if arrow.kind != keyUp || next.kind != keyRune || next.r != 'x' {
		t.Fatalf("keys = %#v %#v", arrow, next)
	}
}

func TestBracketedPasteKeepsTheWholeString(t *testing.T) {
	prev := activeTerm
	t.Cleanup(func() { activeTerm = prev })
	activeTerm = &termSession{notify: make(chan struct{}, 1)}
	payload := []byte{27, '[', '2', '0', '0', '~'}
	payload = append(payload, []byte("páss")...)
	payload = append(payload, 27, '[', '2', '0', '1', '~', 'Z')
	activeTerm.feed(payload)

	pasted, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	if pasted.kind != keyPaste || pasted.text != "páss" {
		t.Fatalf("paste = %#v", pasted)
	}
	next, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	if next.kind != keyRune || next.r != 'Z' {
		t.Fatalf("after paste = %#v", next)
	}
}

func TestUTF8PasswordRune(t *testing.T) {
	prev := activeTerm
	t.Cleanup(func() { activeTerm = prev })
	activeTerm = &termSession{notify: make(chan struct{}, 1)}
	activeTerm.feed([]byte("é"))

	key, err := readKey()
	if err != nil {
		t.Fatal(err)
	}
	if key.kind != keyRune || key.r != 'é' {
		t.Fatalf("key = %#v", key)
	}
}

func TestScreenFitsTheTerminal(t *testing.T) {
	start := time.Now()
	for _, width := range []int{40, 68} {
		screens := [][]string{
			layoutPassword(width, "Wrong password. Try again.", []rune("páss"), 4),
			layoutRename(width, []rune("report.pdf"), 3),
			layoutMenu(width, "Already saved", "report.pdf", []string{"18.20 MB  ·  15 Oct 2026 · 09:19", "/tmp/downloads"}, []string{"Overwrite", "Keep both", "Rename", "Cancel"}, 1),
			layoutSteps(width, 2, "Loading file list", "Notes", 128<<20, 4, 1, []FileNode{{Name: "photo.jpg", S: 2200000}, {Name: "notes.pdf", S: 80000}}),
			layoutProgress(width, "Download", "report.pdf", 18<<20, 64<<20, start),
			layoutStatus(width, "Upload", "Preparing files"),
			inputBox(width, []rune("secret"), 6),
		}
		for _, lines := range screens {
			if len(lines) == 0 {
				t.Fatal("empty screen")
			}
			for _, line := range lines {
				visible := stripANSI(line)
				if got := utf8.RuneCountInString(visible); got > width+2 {
					t.Fatalf("width %d line %d: %q", width, got, visible)
				}
			}
			if !strings.Contains(stripANSI(strings.Join(lines, "\n")), "─") && len(lines) > 3 {
				t.Fatalf("missing rule: %q", stripANSI(strings.Join(lines, "\n")))
			}
		}
	}
}

func TestPasswordScreenShowsWhatWasTyped(t *testing.T) {
	lines := layoutPassword(48, "Wrong password. Try again.", []rune("páss"), 4)
	visible := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"transfer.it", "Password", "Wrong password. Try again.", "páss", "enter", "esc"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q in\n%s", want, visible)
		}
	}
}

func TestDrawStartsEachLineAtColumnZero(t *testing.T) {
	var buf bytes.Buffer
	prev := screenOut
	screenOut = &buf
	t.Cleanup(func() { screenOut = prev })

	draw([]string{"one", "two"})
	got := buf.String()
	want := "\033[H\rone\033[K\r\n\rtwo\033[K\r\n\033[J"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestCleanPastedTextUsesTheFirstLine(t *testing.T) {
	if got := cleanPastedText("secret\r\nmore"); got != "secret" {
		t.Fatalf("got %q", got)
	}
}
