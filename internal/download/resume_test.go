package download

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDownloadResumesAfterShortRead(t *testing.T) {
	payload := bytes.Repeat([]byte("xyz"), 200)
	var resumed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijack unsupported")
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(payload))
			buf.Write(payload[:100])
			buf.Flush()
			conn.Close()
			return
		}
		if r.Header.Get("Range") != "bytes=100-" {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		resumed = true
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 100-%d/%d", len(payload)-1, len(payload)))
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-100))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[100:])
	}))
	defer srv.Close()

	dir := t.TempDir()
	d := &Downloader{
		client:      srv.Client(),
		silent:      true,
		retryBudget: time.Minute,
		retryDelay:  5 * time.Millisecond,
	}
	res, err := d.downloadFile(srv.URL, "notes.bin", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("download did not resume")
	}
	got, err := os.ReadFile(res.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %d bytes, want %d", len(got), len(payload))
	}
	if res.path != filepath.Join(dir, "notes.bin") {
		t.Fatalf("path = %s", res.path)
	}
	if _, err := os.Stat(res.path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file left behind: %v", err)
	}
}

func TestContentRangeTotal(t *testing.T) {
	n, ok := contentRangeTotal("bytes 100-599/600")
	if !ok || n != 600 {
		t.Fatalf("%d %v", n, ok)
	}
	if _, ok := contentRangeTotal("bytes 0-1/*"); ok {
		t.Fatal("star total should be unknown")
	}
}
