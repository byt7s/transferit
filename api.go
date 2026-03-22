package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
)

type Downloader struct {
	client  *http.Client
	seqID   int
	verbose bool
	silent  bool
	pw      string // derived password for protected transfers
}

func NewDownloader(verbose, silent bool) *Downloader {
	return &Downloader{
		client:  &http.Client{},
		seqID:   rand.Intn(0xFFFFFFFF),
		verbose: verbose && !silent,
		silent:  silent,
		pw:      "",
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

func (d *Downloader) apiRequestBT7(payload interface{}, xh string) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	reqURL, _ := url.Parse(bt7API + "/cs")
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

	req, err := http.NewRequest("POST", reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	d.log("  response: %s", truncate(string(respBody), 300))
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

func (d *Downloader) getTransferInfo(xh string) (*TransferInfo, error) {
	d.log("Getting transfer info for xh=%s", xh)

	raw, err := d.apiRequestBT7([]map[string]string{{"a": "xi", "xh": xh}}, "")
	if err != nil {
		return nil, err
	}

	first, err := unwrapArray(raw)
	if err != nil {
		return nil, err
	}

	if errCode, ok := checkAPIError(first); ok {
		return nil, apiError(errCode)
	}

	var info TransferInfo
	if err := json.Unmarshal(first, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

var errPasswordRequired = fmt.Errorf("password required")

func (d *Downloader) fetchFileList(xh string, password string) ([]FileNode, error) {
	d.log("Fetching file list for xh=%s", xh)

	payload := map[string]interface{}{"a": "f", "c": 1, "r": 1}
	if password != "" {
		d.pw = derivePassword(xh, password)
		d.log("Derived password: %s", d.pw)
	}
	if d.pw == "" {
		payload["xnc"] = 1
	}
	raw, err := d.apiRequestBT7([]map[string]interface{}{payload}, xh)
	if err != nil {
		return nil, err
	}

	first, err := unwrapArray(raw)
	if err != nil {
		return nil, err
	}

	if errCode, ok := checkAPIError(first); ok {
		if errCode == -14 {
			return nil, errPasswordRequired
		}
		return nil, apiError(errCode)
	}

	var flr FileListResponse
	if err := json.Unmarshal(first, &flr); err != nil {
		return nil, err
	}

	d.log("Got %d node(s)", len(flr.F))
	return flr.F, nil
}

func unwrapArray(raw json.RawMessage) (json.RawMessage, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return raw, nil
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("empty API response")
	}
	return arr[0], nil
}

func checkAPIError(raw json.RawMessage) (int, bool) {
	var code int
	if err := json.Unmarshal(raw, &code); err == nil && code < 0 {
		return code, true
	}
	return 0, false
}

func apiError(code int) error {
	errors := map[int]string{
		-2:  "EARGS - Invalid arguments",
		-9:  "ENOENT - Transfer not found or expired",
		-11: "EACCESS - Access denied",
		-14: "EACCESS - Incorrect password",
		-16: "EEXPIRED - Transfer has expired",
		-17: "EOVERQUOTA - Over quota",
	}
	if msg, ok := errors[code]; ok {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("UNKNOWN - %d", code)
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
