package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"transferit/internal/api"
	"transferit/internal/crypto"
)

func (u *Uploader) createTransfer(ctx context.Context, name string) (xh, root string, err error) {
	key, err := crypto.RandomA32(4)
	if err != nil {
		return "", "", err
	}
	at, err := crypto.EncryptAttr(crypto.FolderAttrJSON(name, time.Now().Unix()), key)
	if err != nil {
		return "", "", err
	}
	raw, err := u.call(ctx, api.BT7, true, []map[string]string{{
		"a":  "xn",
		"at": at,
		"k":  crypto.Base64URLEncode(crypto.A32ToBytes(key)),
	}})
	if err != nil {
		return "", "", err
	}
	body, err := ensureOK(raw)
	if err != nil {
		return "", "", fmt.Errorf("creating transfer: %w", err)
	}
	var found []string
	collectStrings(body, &found)
	for _, s := range found {
		switch len(s) {
		case 12:
			xh = s
		case 8:
			root = s
		}
	}
	if xh == "" || root == "" {
		return "", "", fmt.Errorf("unexpected transfer response: %s", api.Truncate(string(raw), 180))
	}
	return xh, root, nil
}

func (u *Uploader) finishTransfer(ctx context.Context, xh, title, password string, expiry int) error {
	req := map[string]any{
		"a":  "xm",
		"xh": xh,
		"t":  crypto.Base64URLEncode([]byte(title)),
		"e":  expiry,
	}
	if password != "" {
		req["pw"] = crypto.DerivePassword(xh, password)
	}
	raw, err := u.call(ctx, api.BT7, true, []map[string]any{req})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("setting transfer options: %w", err)
	}
	raw, err = u.call(ctx, api.BT7, true, []map[string]string{{"a": "xc", "xh": xh}})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("closing transfer: %w", err)
	}
	return nil
}

func (u *Uploader) deleteTransfer(ctx context.Context, xh string) {
	_, _ = u.call(ctx, api.BT7, true, []map[string]string{{"a": "xd", "xh": xh}})
}

type uploadPool struct {
	host  string
	uri   string
	limit int64
}

func (u *Uploader) uploadPools(ctx context.Context) ([]uploadPool, error) {
	raw, err := u.call(ctx, api.G, false, []map[string]string{{"a": "usc"}})
	if err != nil {
		return nil, err
	}
	body, err := ensureOK(raw)
	if err != nil {
		return nil, fmt.Errorf("upload servers: %w", err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("upload servers: %s", api.Truncate(string(body), 180))
	}
	// A single pool is [host, uri, limit]. A list of pools is [[host, uri, limit], ...].
	if len(rows) > 0 {
		if _, ok := jsonString(rows[0]); ok {
			pool, err := parsePool(rows)
			if err != nil {
				return nil, err
			}
			return []uploadPool{pool}, nil
		}
	}
	pools := make([]uploadPool, 0, len(rows))
	for _, row := range rows {
		var parts []json.RawMessage
		if err := json.Unmarshal(row, &parts); err != nil {
			return nil, fmt.Errorf("upload server entry: %s", api.Truncate(string(row), 120))
		}
		pool, err := parsePool(parts)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("no upload servers available")
	}
	return pools, nil
}

func parsePool(parts []json.RawMessage) (uploadPool, error) {
	if len(parts) < 2 {
		return uploadPool{}, fmt.Errorf("short upload server entry")
	}
	host, ok := jsonString(parts[0])
	if !ok || host == "" {
		return uploadPool{}, fmt.Errorf("upload server missing host")
	}
	uri, ok := jsonString(parts[1])
	if !ok || uri == "" {
		return uploadPool{}, fmt.Errorf("upload server missing path")
	}
	var limit int64
	if len(parts) >= 3 {
		if err := json.Unmarshal(parts[2], &limit); err != nil {
			return uploadPool{}, fmt.Errorf("upload server size: %w", err)
		}
	}
	return uploadPool{host: host, uri: strings.TrimPrefix(uri, "/"), limit: limit}, nil
}

func choosePool(pools []uploadPool, size int64) (uploadPool, error) {
	var unlimited *uploadPool
	for i := range pools {
		if pools[i].limit <= 0 {
			if unlimited == nil {
				unlimited = &pools[i]
			}
			continue
		}
		if size <= pools[i].limit {
			return pools[i], nil
		}
	}
	if unlimited != nil {
		return *unlimited, nil
	}
	if len(pools) > 0 {
		return pools[len(pools)-1], nil
	}
	return uploadPool{}, fmt.Errorf("no upload server available")
}
