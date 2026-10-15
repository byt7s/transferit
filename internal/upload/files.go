package upload

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Item struct {
	Path    string
	Name    string
	Title   string
	Cleanup func()
}

func Prepare(paths []string) ([]Item, error) {
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
			zipPath, zipName, err := zipDirectory(path)
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

func zipDirectory(dir string) (string, string, error) {
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "folder"
	}
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
		_, copyErr := io.Copy(w, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
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
	ok = true
	return f.Name(), base + ".zip", nil
}
