package upload

import (
	"encoding/json"

	"transferit/internal/api"
)

func ensureOK(raw json.RawMessage) (json.RawMessage, error) {
	first, err := api.Unwrap(raw)
	if err != nil {
		return nil, err
	}
	if code, ok := api.Code(first); ok {
		return nil, api.Error(code)
	}
	var arr []json.RawMessage
	if json.Unmarshal(first, &arr) == nil && len(arr) > 0 {
		if code, ok := jsonInt(arr[0]); ok {
			if code < 0 {
				return nil, api.Error(code)
			}
			if code == 0 && len(arr) > 1 {
				return arr[1], nil
			}
		}
	}
	return first, nil
}

func jsonInt(raw json.RawMessage) (int, bool) {
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return n, true
}

func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func collectStrings(raw json.RawMessage, out *[]string) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		*out = append(*out, s)
		return
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return
	}
	for _, el := range arr {
		collectStrings(el, out)
	}
}
