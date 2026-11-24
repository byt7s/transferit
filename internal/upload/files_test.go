package upload

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareReportsZipProgress(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "photos")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("a"), 600*1024)
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var reports int
	var lastName string
	var lastDone, lastTotal int64
	items, err := Prepare([]string{dir}, func(name string, done, total int64) {
		reports++
		lastName, lastDone, lastTotal = name, done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	defer items[0].Cleanup()

	if len(items) != 1 || items[0].Name != "photos.zip" {
		t.Fatalf("items = %#v", items)
	}
	want := int64(len(payload) + len("hello"))
	if lastName != "photos.zip" || lastTotal != want || lastDone != want {
		t.Fatalf("last report = %s %d/%d, want photos.zip %d/%d", lastName, lastDone, lastTotal, want, want)
	}
	if reports < 2 {
		t.Fatalf("expected progress reports, got %d", reports)
	}

	r, err := zip.OpenReader(items[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.File) != 3 {
		t.Fatalf("zip entries = %d, want 3", len(r.File))
	}
}
