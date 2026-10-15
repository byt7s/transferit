package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxDownloadRunes = 80

func sanitizeDownloadName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}

	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimRight(b.String(), " .")
	name = limitRunes(name, maxDownloadRunes)
	name = strings.TrimRight(name, " .")
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	if windowsReserved(name) {
		name = "_" + name
		name = limitRunes(name, maxDownloadRunes)
	}
	return name
}

func windowsReserved(name string) bool {
	stem := name
	if i := strings.IndexByte(name, '.'); i >= 0 {
		stem = name[:i]
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func limitRunes(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n])
}

func numberedName(filename string, n int) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return sanitizeDownloadName(fmt.Sprintf("%s (%d)%s", base, n, ext))
}

func availablePath(dir, filename string) (string, error) {
	filename = sanitizeDownloadName(filename)
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	for i := 1; i < 10000; i++ {
		path = filepath.Join(dir, numberedName(filename, i))
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no free filename for %s", filename)
}
