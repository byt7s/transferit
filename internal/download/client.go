// Package download fetches files from a transfer.it link.
package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"transferit/internal/api"
	"transferit/internal/crypto"
	"transferit/internal/retry"
	"transferit/internal/ui"
)

func init() {
	ui.OnDiscard = discardPartial
}

type Downloader struct {
	client      *http.Client
	seqID       int
	verbose     bool
	silent      bool
	Pretty      bool
	pw          string
	retryBudget time.Duration
	retryDelay  time.Duration
}

func New(verbose, silent bool) *Downloader {
	return &Downloader{
		client:  &http.Client{Transport: retry.NewTransport()},
		seqID:   rand.Intn(0xFFFFFFFF),
		verbose: verbose && !silent,
		silent:  silent,
	}
}

func (d *Downloader) nextID() int {
	d.seqID++
	return d.seqID
}

func (d *Downloader) log(format string, args ...interface{}) {
	if d.verbose {
		fmt.Printf("  [*] "+format+"\n", args...)
	}
}

func (d *Downloader) apiRequest(payload interface{}, xh string) (json.RawMessage, error) {
	watch := d.newWatch()
	delay := watch.Step()
	for {
		raw, err := d.apiRequestOnce(payload, xh)
		if err == nil {
			return raw, nil
		}
		if !retry.IsTemporary(err) {
			return nil, err
		}
		if err := d.waitNet(watch, &delay); err != nil {
			return nil, err
		}
	}
}

func (d *Downloader) apiRequestOnce(payload interface{}, xh string) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	reqURL, _ := url.Parse(api.BT7 + "/cs")
	q := reqURL.Query()
	q.Set("id", fmt.Sprintf("%d", d.nextID()))
	q.Set("v", "3")
	q.Set("lang", "en")
	q.Set("domain", "transferit")
	q.Set("bc", "1")
	if xh != "" {
		q.Set("x", xh)
	}
	if d.pw != "" {
		q.Set("pw", d.pw)
	}
	reqURL.RawQuery = q.Encode()

	d.log("POST %s", reqURL)
	d.log("  payload: %s", string(body))

	reqCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	api.SetHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, retry.StatusError{Code: resp.StatusCode}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	d.log("  response: %s", api.Truncate(string(respBody), 300))
	return json.RawMessage(respBody), nil
}

func (d *Downloader) extractHandle(input string) (string, error) {
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]{8,16}$`, input); matched {
		return input, nil
	}
	re := regexp.MustCompile(`transfer\.it/t/([A-Za-z0-9_-]+)`)
	if m := re.FindStringSubmatch(input); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("could not extract transfer handle from: %s", input)
}

func (d *Downloader) getTransferInfo(xh string) (*api.TransferInfo, error) {
	d.log("Getting transfer info for xh=%s", xh)

	raw, err := d.apiRequest([]map[string]string{{"a": "xi", "xh": xh}}, "")
	if err != nil {
		return nil, err
	}

	first, err := api.Unwrap(raw)
	if err != nil {
		return nil, err
	}

	if errCode, ok := api.Code(first); ok {
		return nil, api.Error(errCode)
	}

	var info api.TransferInfo
	if err := json.Unmarshal(first, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

var ErrPasswordRequired = fmt.Errorf("password required")

func (d *Downloader) fetchFileList(xh string, password string) ([]api.FileNode, error) {
	d.log("Fetching file list for xh=%s", xh)

	payload := map[string]interface{}{"a": "f", "c": 1, "r": 1}
	if password != "" {
		d.pw = crypto.DerivePassword(xh, password)
		d.log("Derived password: %s", d.pw)
	}
	if d.pw == "" {
		payload["xnc"] = 1
	}
	raw, err := d.apiRequest([]map[string]interface{}{payload}, xh)
	if err != nil {
		return nil, err
	}

	first, err := api.Unwrap(raw)
	if err != nil {
		return nil, err
	}

	if errCode, ok := api.Code(first); ok {
		if errCode == -14 {
			return nil, ErrPasswordRequired
		}
		return nil, api.Error(errCode)
	}

	var flr api.FileListResponse
	if err := json.Unmarshal(first, &flr); err != nil {
		return nil, err
	}

	d.log("Got %d node(s)", len(flr.F))
	return flr.F, nil
}
