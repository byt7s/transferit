package upload

import (
	"context"
	"fmt"
	"os"
	"time"

	"transferit/internal/api"
	"transferit/internal/ui"
)

type Options struct {
	Title    string
	Password string
	Expiry   int
}

type Result struct {
	Link     string
	Password string
	Expiry   int
	Title    string
	Files    int
	Bytes    int64
}

func (u *Uploader) Run(ctx context.Context, items []Item, opt Options) (*Result, error) {
	if err := u.createSession(ctx); err != nil {
		return nil, err
	}
	xh, root, err := u.createTransfer(ctx, opt.Title)
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			u.deleteTransfer(context.Background(), xh)
		}
	}()

	if u.Pretty {
		ui.Draw(ui.LayoutStatus(ui.ContentWidth(), "Upload", "Opening transfer"))
	}

	var total int64
	for _, item := range items {
		info, err := os.Stat(item.Path)
		if err != nil {
			return nil, err
		}
		total += info.Size()
	}
	var sent int64
	start := time.Now()
	for _, item := range items {
		info, err := os.Stat(item.Path)
		if err != nil {
			return nil, err
		}
		fileStart := sent
		if !u.silent {
			u.showProgress(item.Name, sent, total, start)
		}
		err = u.putFile(ctx, root, item.Path, item.Name, func(n int64) {
			sent += n
			if !u.silent {
				u.showProgress(item.Name, sent, total, start)
			}
		})
		if err != nil {
			if !u.silent && !u.Pretty {
				fmt.Fprintln(os.Stderr)
			}
			return nil, err
		}
		if sent < fileStart+info.Size() {
			sent = fileStart + info.Size()
		}
		if !u.silent {
			u.showProgress(item.Name, sent, total, start)
			if !u.Pretty {
				fmt.Fprintln(os.Stderr)
			}
		}
	}

	if err := u.finishTransfer(ctx, xh, opt.Title, opt.Password, opt.Expiry); err != nil {
		return nil, err
	}
	closed = true
	return &Result{
		Link:     api.Web + "/t/" + xh,
		Password: opt.Password,
		Expiry:   opt.Expiry,
		Title:    opt.Title,
		Files:    len(items),
		Bytes:    total,
	}, nil
}

func (u *Uploader) showProgress(name string, done, total int64, start time.Time) {
	if u.silent {
		return
	}
	u.paint(func() {
		if done < total && time.Since(u.lastPaint) < 100*time.Millisecond {
			return
		}
		u.lastPaint = time.Now()
		ui.ShowProgress(u.Pretty, "Upload", name, done, total, start)
	})
}

func (u *Uploader) showStatus(title, detail string) {
	if u.silent {
		return
	}
	u.paint(func() {
		u.lastPaint = time.Time{}
		ui.ShowStatus(u.Pretty, title, detail)
	})
}

func (u *Uploader) paint(fn func()) {
	u.uiMu.Lock()
	defer u.uiMu.Unlock()
	fn()
}
