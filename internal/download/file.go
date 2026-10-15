package download

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"transferit/internal/api"
	"transferit/internal/ui"
)

type downloadResult struct {
	filename string
	path     string
	size     int64
	duration time.Duration
}

var ErrAborted = fmt.Errorf("download aborted by user")

var (
	downloadMu          sync.Mutex
	currentDownloadPath string
)

func setCurrentDownload(path string) {
	downloadMu.Lock()
	currentDownloadPath = path
	downloadMu.Unlock()
}

func discardPartial() {
	downloadMu.Lock()
	path := currentDownloadPath
	currentDownloadPath = ""
	downloadMu.Unlock()
	if path != "" {
		_ = os.Remove(path)
	}
}

func (d *Downloader) downloadURL(xh, nodeHandle, filename string) string {
	u := fmt.Sprintf("%s/cs/g?x=%s&n=%s&fn=%s", api.BT7, xh, nodeHandle, url.QueryEscape(filename))
	if d.pw != "" {
		u += "&pw=" + url.QueryEscape(d.pw)
	}
	return u
}

func (d *Downloader) downloadFile(dlURL, filename, outputDir string) (*downloadResult, error) {
	filename = sanitizeDownloadName(filename)
	outputPath := filepath.Join(outputDir, filename)

	if fileInfo, err := os.Stat(outputPath); err == nil {
		if d.silent {
			var pathErr error
			outputPath, pathErr = availablePath(outputDir, filename)
			if pathErr != nil {
				return nil, pathErr
			}
			filename = filepath.Base(outputPath)
		} else {
			choice, newName, askErr := ui.AskConflict(d.Pretty, outputPath, fileInfo, filename)
			if askErr != nil {
				return nil, askErr
			}
			switch choice {
			case ui.Overwrite:
				_ = os.Remove(outputPath)
				_ = os.Remove(outputPath + ".part")
			case ui.RenameAuto:
				var pathErr error
				outputPath, pathErr = availablePath(outputDir, filename)
				if pathErr != nil {
					return nil, pathErr
				}
				filename = filepath.Base(outputPath)
			case ui.RenameManual:
				newName = sanitizeDownloadName(newName)
				outputPath = filepath.Join(outputDir, newName)
				filename = newName
			case ui.Abort:
				return nil, ErrAborted
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	d.log("Downloading to: %s", outputPath)
	started := time.Now()
	size, err := d.saveURL(dlURL, outputPath, filename, started)
	if err != nil {
		return nil, err
	}
	return &downloadResult{
		filename: filename,
		path:     outputPath,
		size:     size,
		duration: time.Since(started),
	}, nil
}
