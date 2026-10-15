package download

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeDownloadName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"report.pdf", "report.pdf"},
		{"a/b/c.txt", "c.txt"},
		{`a\b\c.txt`, "c.txt"},
		{"../../etc/passwd", "passwd"},
		{"..\\..\\Windows\\system32", "system32"},
		{"con", "_con"},
		{"CON.txt", "_CON.txt"},
		{"con.TXT", "_con.TXT"},
		{"nul", "_nul"},
		{"aux.log", "_aux.log"},
		{"com1", "_com1"},
		{"lpt9.txt", "_lpt9.txt"},
		{"foo:bar", "foo_bar"},
		{`a*b?c"d<e>f|g`, "a_b_c_d_e_f_g"},
		{" trailing ", "trailing"},
		{"dot.", "dot"},
		{"file.txt.", "file.txt"},
		{"", "download"},
		{".", "download"},
		{"..", "download"},
		{"///", "download"},
		{"normal (1).zip", "normal (1).zip"},
		{".gitignore", ".gitignore"},
		{"emoji 🔒.txt", "emoji 🔒.txt"},
		{"C:foo", "C_foo"},
	}
	for _, tc := range cases {
		if got := sanitizeDownloadName(tc.in); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeDownloadNameLimitsLength(t *testing.T) {
	long := strings.Repeat("a", 200) + ".txt"
	got := sanitizeDownloadName(long)
	if len([]rune(got)) > maxDownloadRunes {
		t.Fatalf("len %d", len([]rune(got)))
	}
	if !strings.HasPrefix(got, "aaa") {
		t.Fatalf("got %q", got)
	}
}

func TestAvailablePath(t *testing.T) {
	dir := t.TempDir()
	first, err := availablePath(dir, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(dir, "notes.txt") {
		t.Fatalf("first = %s", first)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := availablePath(dir, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if second != filepath.Join(dir, "notes (1).txt") {
		t.Fatalf("second = %s", second)
	}

	unsafe, err := availablePath(dir, `..\..\con.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if unsafe != filepath.Join(dir, "_con.txt") {
		t.Fatalf("unsafe = %s", unsafe)
	}
}
