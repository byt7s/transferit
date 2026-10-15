package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"transferit/internal/api"
	"transferit/internal/retry"
	"transferit/internal/ui"
)

var errStalled = errors.New("download stalled")

func (d *Downloader) newWatch() *retry.Watch {
	watch := retry.NewWatch()
	if d.retryBudget > 0 {
		watch.Budget = d.retryBudget
	}
	if d.retryDelay > 0 {
		watch.Floor = d.retryDelay
		watch.MaxDelay = d.retryDelay
	}
	return watch
}

func (d *Downloader) showStatus(detail string) {
	if d.silent {
		return
	}
	ui.ShowStatus(d.Pretty, "Download", detail)
}

func (d *Downloader) waitNet(watch *retry.Watch, delay *time.Duration) error {
	if err := watch.Arm(); err != nil {
		return err
	}
	watch.Tell(func() { d.showStatus(retry.WaitNotice) })
	err := retry.Sleep(context.Background(), *delay)
	cap := watch.Cap()
	if *delay < cap {
		*delay *= 2
		if *delay > cap {
			*delay = cap
		}
	}
	return err
}

func (d *Downloader) saveURL(dlURL, outputPath, filename string, start time.Time) (int64, error) {
	partPath := outputPath + ".part"
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	setCurrentDownload(partPath)
	defer setCurrentDownload("")

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	have := info.Size()
	high := have
	expect := int64(-1)
	watch := d.newWatch()
	delay := watch.Step()
	painted := false

	for {
		req, err := http.NewRequest("GET", dlURL, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("User-Agent", api.UserAgent)
		req.Header.Set("Origin", api.Web)
		req.Header.Set("Referer", api.Web+"/")
		if have > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		}
		resp, err := d.client.Do(req)
		if err != nil {
			if !retry.IsTemporary(err) {
				return 0, err
			}
			if err := d.waitNet(watch, &delay); err != nil {
				return 0, err
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusRequestedRangeNotSatisfiable:
			resp.Body.Close()
			if have == 0 {
				return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			if err := f.Truncate(0); err != nil {
				return 0, err
			}
			have = 0
			high = 0
			expect = -1
			continue
		case http.StatusOK:
			if have > 0 {
				if err := f.Truncate(0); err != nil {
					resp.Body.Close()
					return 0, err
				}
				have = 0
			}
		case http.StatusPartialContent:
		default:
			resp.Body.Close()
			status := retry.StatusError{Code: resp.StatusCode}
			if !retry.IsTemporary(status) {
				return 0, status
			}
			if err := d.waitNet(watch, &delay); err != nil {
				return 0, err
			}
			continue
		}

		if total, ok := contentRangeTotal(resp.Header.Get("Content-Range")); ok {
			expect = total
		} else if resp.ContentLength >= 0 {
			if resp.StatusCode == http.StatusPartialContent {
				expect = have + resp.ContentLength
			} else {
				expect = resp.ContentLength
			}
		}

		before := have
		n, readErr := writeBody(resp.Body, f, have, func(got int64) {
			if d.silent {
				return
			}
			total := expect
			if total < 0 {
				total = 0
			}
			ui.ShowProgress(d.Pretty, "Download", filename, before+got, total, start)
			if !d.Pretty {
				painted = true
			}
		})
		resp.Body.Close()
		have += n
		if have > high {
			high = have
			watch.Mark()
		}
		if readErr == nil && (expect < 0 || have >= expect) {
			break
		}
		if readErr != nil && !errors.Is(readErr, errStalled) && !retry.IsTemporary(readErr) {
			return 0, readErr
		}
		_ = f.Sync()
		if err := d.waitNet(watch, &delay); err != nil {
			return 0, err
		}
	}

	if err := f.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(partPath, outputPath); err != nil {
		return 0, err
	}
	if painted {
		fmt.Fprintln(os.Stderr)
	}
	return have, nil
}

func writeBody(body io.Reader, f *os.File, offset int64, onData func(int64)) (int64, error) {
	buf := make([]byte, 1<<20)
	var wrote int64
	for {
		n, err := readIdle(body, buf, time.Minute)
		if n > 0 {
			if _, werr := f.WriteAt(buf[:n], offset+wrote); werr != nil {
				return wrote, werr
			}
			wrote += int64(n)
			if onData != nil {
				onData(wrote)
			}
		}
		if err != nil {
			if err == io.EOF {
				return wrote, nil
			}
			return wrote, err
		}
	}
}

func readIdle(r io.Reader, buf []byte, idle time.Duration) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := r.Read(buf)
		ch <- result{n, err}
	}()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-timer.C:
		return 0, errStalled
	}
}

func contentRangeTotal(header string) (int64, bool) {
	_, total, ok := strings.Cut(header, "/")
	if !ok || total == "" || total == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
