package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type downloadResult struct {
	filename string
	path     string
	size     int64
	duration time.Duration
}

var currentDownloadPath string

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
	outputPath := filepath.Join(outputDir, filename)

	if fileInfo, err := os.Stat(outputPath); err == nil {
		if !d.silent {
			choice, newName := d.showFileConflictMenu(outputPath, fileInfo, filename)
			switch choice {
			case choiceOverwrite:
				// Continue with same path, will overwrite
			case choiceRenameAuto:
				ext := filepath.Ext(filename)
				base := strings.TrimSuffix(filename, ext)
				for i := 1; ; i++ {
					outputPath = filepath.Join(outputDir, fmt.Sprintf("%s (%d)%s", base, i, ext))
					if _, err := os.Stat(outputPath); os.IsNotExist(err) {
						break
					}
				}
			case choiceRenameManual:
				outputPath = filepath.Join(outputDir, newName)
				filename = newName
			case choiceAbort:
				return nil, errAborted
			}
		} else {
			// Silent mode: auto-rename
			ext := filepath.Ext(filename)
			base := strings.TrimSuffix(filename, ext)
			for i := 1; ; i++ {
				outputPath = filepath.Join(outputDir, fmt.Sprintf("%s (%d)%s", base, i, ext))
				if _, err := os.Stat(outputPath); os.IsNotExist(err) {
					break
				}
			}
		}
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
	currentDownloadPath = outputPath
	defer func() {
		f.Close()
		currentDownloadPath = ""
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
			if !d.silent && total > 0 {
				if firstRender {
					clearScreen()
					firstRender = false
				}
				d.renderTUI(filename, downloaded, total, startTime)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
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

	if !d.silent {
		enterAltScreen()
		hideCursor()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt)
		go func() {
			<-sigChan
			showCursor()
			exitAltScreen()
			if currentDownloadPath != "" {
				os.Remove(currentDownloadPath)
			}
			fmt.Print("Download aborted.\r\n")
			os.Exit(0)
		}()

		defer func() {
			signal.Stop(sigChan)
			showCursor()
			exitAltScreen()
			for _, r := range results {
				fmt.Printf("Saved: %s (%s) in %s\n", r.filename, formatSize(r.size), formatDuration(r.duration))
			}
		}()
	}

	if !d.silent {
		d.renderStep(1, "Loading transfer info...", nil, "", nil)
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

	if !d.silent {
		d.renderStep(2, "Loading file list...", info, title, nil)
	}

	rawNodes, err := d.fetchFileList(xh, password)
	if err == errPasswordRequired {
		if d.silent {
			return fmt.Errorf("transfer requires password")
		}
		for {
			password = d.showPasswordInput()
			if password == "" {
				return errAborted
			}
			d.renderStep(2, "Verifying password...", info, title, nil)
			rawNodes, err = d.fetchFileList(xh, password)
			if err == errPasswordRequired {
				continue
			}
			break
		}
	}
	if err != nil {
		return err
	}

	if !d.silent {
		d.renderStep(3, "Reading file list...", info, title, nil)
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

	if !d.silent {
		d.renderStep(3, "Starting download...", info, title, files)
		time.Sleep(500 * time.Millisecond)
	}

	if listOnly {
		return nil
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	if hasZip && numFiles > 1 {
		zipName := xh + ".zip"
		if title != "" {
			safe := regexp.MustCompile(`[<>:"/\\|?*]`).ReplaceAllString(title, "_")
			zipName = safe + ".zip"
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
