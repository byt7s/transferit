package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type downloadResult struct {
	filename string
	path     string
	size     int64
	duration time.Duration
}

var (
	downloadMu          sync.Mutex
	currentDownloadPath string
)

func setCurrentDownload(path string) {
	downloadMu.Lock()
	currentDownloadPath = path
	downloadMu.Unlock()
}

func discardPartialDownload() {
	downloadMu.Lock()
	path := currentDownloadPath
	currentDownloadPath = ""
	downloadMu.Unlock()
	if path != "" {
		_ = os.Remove(path)
	}
}

func (d *Downloader) decodeNodes(nodes []FileNode) []FileNode {
	decoded := make([]FileNode, 0, len(nodes))
	for _, n := range nodes {
		if n.T == 1 {
			n.Type = "folder"
		} else {
			n.Type = "file"
		}

		if n.K != "" && n.A != "" {
			keyA32, err := base64ToA32(n.K)
			if err != nil {
				n.Name = fmt.Sprintf("[key-error-%s]", n.H)
				decoded = append(decoded, n)
				continue
			}

			if len(keyA32) == 4 {
				keyA32 = append(keyA32, keyA32...)
			}

			if len(keyA32) >= 8 {
				attrs, err := decryptAttr(n.A, keyA32[:8])
				if err == nil {
					if name, ok := attrs["n"].(string); ok {
						n.Name = name
					} else {
						n.Name = fmt.Sprintf("[encrypted-%s]", n.H)
					}
				} else {
					n.Name = fmt.Sprintf("[encrypted-%s]", n.H)
				}
			} else {
				n.Name = fmt.Sprintf("[short-key-%s]", n.H)
			}
		} else if n.Name == "" {
			n.Name = fmt.Sprintf("[no-attrs-%s]", n.H)
		}

		decoded = append(decoded, n)
	}
	return decoded
}

func (d *Downloader) downloadURL(xh, nodeHandle, filename string) string {
	u := fmt.Sprintf("%s/cs/g?x=%s&n=%s&fn=%s", bt7API, xh, nodeHandle, url.QueryEscape(filename))
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
			choice, newName, askErr := d.askConflict(outputPath, fileInfo, filename)
			if askErr != nil {
				return nil, askErr
			}
			switch choice {
			case choiceOverwrite:
			case choiceRenameAuto:
				var pathErr error
				outputPath, pathErr = availablePath(outputDir, filename)
				if pathErr != nil {
					return nil, pathErr
				}
				filename = filepath.Base(outputPath)
			case choiceRenameManual:
				newName = sanitizeDownloadName(newName)
				outputPath = filepath.Join(outputDir, newName)
				filename = newName
			case choiceAbort:
				return nil, errAborted
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	d.log("Downloading to: %s", outputPath)

	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", transferIT)
	req.Header.Set("Referer", transferIT+"/")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	f, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	setCurrentDownload(outputPath)
	defer func() {
		f.Close()
		setCurrentDownload("")
	}()

	buf := make([]byte, 1024*1024)
	var downloaded int64
	startTime := time.Now()
	firstRender := true

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return nil, err
			}
			downloaded += int64(n)
			if !d.silent {
				if d.pretty {
					d.renderTUI(filename, downloaded, total, startTime)
				} else {
					printTransferProgress(filename, downloaded, total, startTime)
				}
				firstRender = false
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	if !d.silent && !d.pretty && !firstRender {
		fmt.Fprintln(os.Stderr)
	}

	elapsed := time.Since(startTime)
	return &downloadResult{
		filename: filename,
		path:     outputPath,
		size:     downloaded,
		duration: elapsed,
	}, nil
}

func (d *Downloader) Run(input, outputDir, password string, listOnly bool) error {
	xh, err := d.extractHandle(input)
	if err != nil {
		return err
	}

	var results []*downloadResult
	defer func() {
		if !d.silent && !listOnly {
			printSaved(results)
		}
	}()

	if !d.silent && !listOnly && stdinIsTerminal() {
		if err := enterUI(); err == nil {
			d.pretty = true
			defer exitUI()
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		abortDownload()
	}()

	if d.pretty {
		d.renderStep(1, "Loading transfer info", nil, "", nil, nil)
	}

	info, err := d.getTransferInfo(xh)
	if err != nil {
		return err
	}

	var title string
	if info.T != "" {
		if b, err := base64URLDecode(info.T); err == nil {
			title = string(b)
		}
	}

	var numFiles int64
	if len(info.Size) > 1 {
		numFiles = info.Size[1]
	}

	hasZip := info.Z != ""

	if d.pretty {
		d.renderStep(2, "Loading file list", info, title, nil, nil)
	}

	rawNodes, err := d.fetchFileList(xh, password)
	warning := ""
	if err == errPasswordRequired {
		if d.silent {
			return fmt.Errorf("transfer requires a password; pass -p")
		}
		for {
			var askErr error
			password, askErr = d.askPassword(warning)
			if askErr != nil {
				return askErr
			}
			if password == "" {
				return errAborted
			}
			if d.pretty {
				d.renderStep(2, "Verifying password", info, title, nil, nil)
			}
			rawNodes, err = d.fetchFileList(xh, password)
			if err == errPasswordRequired {
				warning = "Wrong password. Try again."
				continue
			}
			break
		}
	}
	if err != nil {
		return err
	}

	nodes := d.decodeNodes(rawNodes)

	var files, folders []FileNode
	for _, n := range nodes {
		if n.Type == "file" {
			files = append(files, n)
		} else {
			folders = append(folders, n)
		}
	}

	if listOnly {
		if !d.silent {
			var totalBytes int64
			if len(info.Size) > 0 {
				totalBytes = info.Size[0]
			}
			printPlainList(title, totalBytes, files, folders)
		}
		return nil
	}

	if d.pretty {
		d.renderStep(3, "Starting download", info, title, files, folders)
		time.Sleep(450 * time.Millisecond)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	if hasZip && numFiles > 1 {
		zipName := sanitizeDownloadName(xh + ".zip")
		if strings.TrimSpace(title) != "" {
			zipName = sanitizeDownloadName(title + ".zip")
		}
		dlURL := d.downloadURL(xh, info.Z, zipName)
		result, err := d.downloadFile(dlURL, zipName, outputDir)
		if err != nil {
			return err
		}
		results = append(results, result)
	} else if len(files) == 1 {
		f := files[0]
		name := f.Name
		if name == "" {
			name = fmt.Sprintf("download_%s", f.H)
		}
		dlURL := d.downloadURL(xh, f.H, name)
		result, err := d.downloadFile(dlURL, name, outputDir)
		if err != nil {
			return err
		}
		results = append(results, result)
	} else {
		for _, f := range files {
			name := f.Name
			if name == "" {
				name = fmt.Sprintf("download_%s", f.H)
			}
			dlURL := d.downloadURL(xh, f.H, name)
			result, err := d.downloadFile(dlURL, name, outputDir)
			if err != nil {
				d.log("ERROR downloading %s: %v", name, err)
				continue
			}
			results = append(results, result)
		}
	}

	return nil
}
