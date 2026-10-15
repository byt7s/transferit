package download

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"transferit/internal/api"
	"transferit/internal/crypto"
	"transferit/internal/format"
	"transferit/internal/ui"
)

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

	if !d.silent && !listOnly && ui.StdinIsTerminal() {
		if err := ui.Enter(); err == nil {
			d.Pretty = true
			defer ui.Exit()
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		abort()
	}()

	if d.Pretty {
		ui.RenderStep(true, 1, "Loading transfer info", "", 0, nil, nil)
	}

	info, err := d.getTransferInfo(xh)
	if err != nil {
		return err
	}

	var title string
	if info.T != "" {
		if b, err := crypto.Base64URLDecode(info.T); err == nil {
			title = string(b)
		}
	}

	var numFiles int64
	if len(info.Size) > 1 {
		numFiles = info.Size[1]
	}

	hasZip := info.Z != ""

	if d.Pretty {
		ui.RenderStep(true, 2, "Loading file list", title, transferBytes(info), nil, nil)
	}

	rawNodes, err := d.fetchFileList(xh, password)
	warning := ""
	if err == ErrPasswordRequired {
		if d.silent {
			return fmt.Errorf("transfer requires a password; pass -p")
		}
		for {
			var askErr error
			password, askErr = ui.AskPassword(d.Pretty, warning)
			if askErr != nil {
				return askErr
			}
			if password == "" {
				return ErrAborted
			}
			if d.Pretty {
				ui.RenderStep(true, 2, "Verifying password", title, transferBytes(info), nil, nil)
			}
			rawNodes, err = d.fetchFileList(xh, password)
			if err == ErrPasswordRequired {
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

	var files, folders []api.FileNode
	for _, n := range nodes {
		if n.Type == "file" {
			files = append(files, n)
		} else {
			folders = append(folders, n)
		}
	}

	if listOnly {
		if !d.silent {
			ui.PrintList(title, transferBytes(info), fileViews(files), fileViews(folders))
		}
		return nil
	}

	if d.Pretty {
		ui.RenderStep(true, 3, "Starting download", title, transferBytes(info), fileViews(files), fileViews(folders))
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

func transferBytes(info *api.TransferInfo) int64 {
	if info != nil && len(info.Size) > 0 {
		return info.Size[0]
	}
	return 0
}

func fileViews(nodes []api.FileNode) []ui.File {
	out := make([]ui.File, len(nodes))
	for i, n := range nodes {
		out[i] = ui.File{Name: n.Name, Size: n.S}
	}
	return out
}

func printSaved(results []*downloadResult) {
	if len(results) == 0 {
		return
	}
	fmt.Println()
	for _, result := range results {
		fmt.Printf("  saved  %s\n", result.path)
		fmt.Printf("         %s · %s\n", format.Size(result.size), format.Duration(result.duration))
	}
}

func abort() {
	ui.Exit()
	discardPartial()
	fmt.Fprint(os.Stdout, "Download aborted.\r\n")
	os.Exit(0)
}
