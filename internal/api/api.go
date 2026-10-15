// Package api holds the transfer.it endpoints and the response shapes both
// upload and download share.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	BT7       = "https://bt7.api.mega.co.nz"
	G         = "https://g.api.mega.co.nz"
	Web       = "https://transfer.it"
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
)

type TransferInfo struct {
	Size []int64         `json:"size"`
	T    string          `json:"t"`
	M    string          `json:"m"`
	Z    string          `json:"z"`
	SE   string          `json:"se"`
	PW   json.RawMessage `json:"pw"`
}

type FileNode struct {
	H    string `json:"h"`
	T    int    `json:"t"`
	K    string `json:"k"`
	A    string `json:"a"`
	S    int64  `json:"s"`
	P    string `json:"p"`
	Name string `json:"-"`
	Type string `json:"-"`
}

type FileListResponse struct {
	F []FileNode `json:"f"`
}

func SetHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Origin", Web)
	req.Header.Set("Referer", Web+"/")
}

func Unwrap(raw json.RawMessage) (json.RawMessage, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return raw, nil
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("empty API response")
	}
	return arr[0], nil
}

func Code(raw json.RawMessage) (int, bool) {
	var code int
	if err := json.Unmarshal(raw, &code); err == nil && code < 0 {
		return code, true
	}
	return 0, false
}

func Error(code int) error {
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

func Truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
