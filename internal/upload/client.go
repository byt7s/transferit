// Package upload sends files to transfer.it.
package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"transferit/internal/api"
	"transferit/internal/retry"
)

type Uploader struct {
	client    *http.Client
	seqID     int
	verbose   bool
	silent    bool
	Pretty    bool
	sid       string
	uiMu      sync.Mutex
	lastPaint time.Time
}

func New(verbose, silent bool) *Uploader {
	return &Uploader{
		client:  &http.Client{Transport: retry.NewTransport()},
		seqID:   rand.Intn(0xFFFFFFFF),
		verbose: verbose && !silent,
		silent:  silent,
	}
}

func (u *Uploader) nextID() int {
	u.seqID++
	return u.seqID
}

func (u *Uploader) log(format string, args ...interface{}) {
	if u.verbose {
		fmt.Fprintf(os.Stderr, "  [*] "+format+"\n", args...)
	}
}

func (u *Uploader) call(ctx context.Context, base string, bc bool, payload any) (json.RawMessage, error) {
	watch := retry.NewWatch()
	delay := watch.Step()
	busy := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := u.callOnce(ctx, base, bc, payload)
		if err == nil {
			first, _ := api.Unwrap(raw)
			if code, ok := api.Code(first); ok && code == -3 {
				busy++
				if busy >= 5 {
					return nil, api.Error(code)
				}
				if err := retry.Sleep(ctx, time.Duration(busy)*400*time.Millisecond); err != nil {
					return nil, err
				}
				continue
			}
			return raw, nil
		}
		if !retry.IsTemporary(err) {
			return nil, err
		}
		if err := u.waitNet(ctx, watch, &delay); err != nil {
			return nil, err
		}
	}
}

func (u *Uploader) callOnce(ctx context.Context, base string, bc bool, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("id", fmt.Sprintf("%d", u.nextID()))
	q.Set("v", "3")
	q.Set("lang", "en")
	q.Set("domain", "transferit")
	if u.sid != "" {
		q.Set("sid", u.sid)
	}
	if bc {
		q.Set("bc", "1")
	}
	endpoint := base + "/cs?" + q.Encode()
	u.log("POST %s", endpoint)
	u.log("  payload: %s", api.Truncate(string(body), 500))

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	api.SetHeaders(req)

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	u.log("  response: %s", api.Truncate(string(respBody), 500))
	if resp.StatusCode != http.StatusOK {
		return nil, retry.StatusError{Code: resp.StatusCode, Body: api.Truncate(string(respBody), 180)}
	}
	return json.RawMessage(respBody), nil
}
