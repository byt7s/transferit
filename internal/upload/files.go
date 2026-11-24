package upload

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Item struct {
	Path    string
	Name    string
	Title   string
	Cleanup func()
}

// Progress reports how much of an archive has been written so far.
type Progress func(name string, done, total int64)

func Prepare(paths []string, progress Progress) ([]Item, error) {
	var items []Item
	seen := map[string]int{}
	unique := func(name string) string {
		if seen[name] == 0 {
			seen[name]++
			return name
		}
		seen[name]++
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		return fmt.Sprintf("%s (%d)%s", base, seen[name]-1, ext)
	}
	cleanup := func() {
		for _, item := range items {
			if item.Cleanup != nil {
				item.Cleanup()
			}
		}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			cleanup()
			return nil, err
		}
		if info.IsDir() {
			zipPath, zipName, err := zipDirectory(path, progress)
			if err != nil {
				cleanup()
				return nil, err
			}
			items = append(items, Item{
				Path:    zipPath,
				Name:    unique(zipName),
				Title:   filepath.Base(path),
				Cleanup: func() { os.Remove(zipPath) },
			})
			continue
		}
		if !info.Mode().IsRegular() {
			cleanup()
			return nil, fmt.Errorf("%s is not a file or folder", path)
		}
		name := filepath.Base(path)
		items = append(items, Item{Path: path, Name: unique(name), Title: name})
	}
	return items, nil
}

func zipDirectory(dir string, progress Progress) (string, string, error) {
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "folder"
	}
	zipName := base + ".zip"
	total, err := dirSize(dir)
	if err != nil {
		return "", "", err
	}
	report := func(done int64) {
		if progress != nil {
			progress(zipName, done, total)
		}
	}
	report(0)
	f, err := os.CreateTemp("", "transferit-*.zip")
	if err != nil {
		return "", "", err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	zw := zip.NewWriter(f)
	var done int64
	lastReport := time.Time{}
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		buf := make([]byte, 256*1024)
		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				if _, err := w.Write(buf[:n]); err != nil {
					src.Close()
					return err
				}
				done += int64(n)
				if done >= total || time.Since(lastReport) >= 100*time.Millisecond {
					lastReport = time.Now()
					report(done)
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				src.Close()
				return readErr
			}
		}
		return src.Close()
	})
	closeZip := zw.Close()
	closeFile := f.Close()
	if err != nil {
		return "", "", err
	}
	if closeZip != nil {
		return "", "", closeZip
	}
	if closeFile != nil {
		return "", "", closeFile
	}
	report(done)
	ok = true
	return f.Name(), zipName, nil
}

func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
