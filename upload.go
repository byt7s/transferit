package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type uploadOptions struct {
	title    string
	password string
	expiry   int
}

type uploadResult struct {
	link     string
	password string
	expiry   int
	title    string
	files    int
	bytes    int64
}

type uploadItem struct {
	path    string
	name    string
	title   string
	cleanup func()
}

type Uploader struct {
	client  *http.Client
	seqID   int
	verbose bool
	silent  bool
	pretty  bool
	sid     string
}

func NewUploader(verbose, silent bool) *Uploader {
	return &Uploader{
		client:  &http.Client{},
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
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		raw, err := u.callOnce(ctx, base, bc, payload)
		if err != nil {
			last = err
			continue
		}
		first, _ := unwrapArray(raw)
		if code, ok := checkAPIError(first); ok && code == -3 {
			last = apiError(code)
			continue
		}
		return raw, nil
	}
	if last == nil {
		last = fmt.Errorf("API request failed")
	}
	return nil, last
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
	u.log("  payload: %s", truncate(string(body), 500))

	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", transferIT)
	req.Header.Set("Referer", transferIT+"/")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	u.log("  response: %s", truncate(string(respBody), 500))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 180))
	}
	return json.RawMessage(respBody), nil
}

func ensureOK(raw json.RawMessage) (json.RawMessage, error) {
	first, err := unwrapArray(raw)
	if err != nil {
		return nil, err
	}
	if code, ok := checkAPIError(first); ok {
		return nil, apiError(code)
	}
	var arr []json.RawMessage
	if json.Unmarshal(first, &arr) == nil && len(arr) > 0 {
		if code, ok := jsonInt(arr[0]); ok {
			if code < 0 {
				return nil, apiError(code)
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

func (u *Uploader) createSession(ctx context.Context) error {
	master, err := randomA32(4)
	if err != nil {
		return err
	}
	passwordKey, err := randomA32(4)
	if err != nil {
		return err
	}
	ssc, err := randomA32(4)
	if err != nil {
		return err
	}
	masterBytes := a32ToBytes(master)
	sscBytes := a32ToBytes(ssc)
	encMaster, err := aesEncryptBlock(a32ToBytes(passwordKey), masterBytes)
	if err != nil {
		return err
	}
	encSSC, err := aesEncryptBlock(masterBytes, sscBytes)
	if err != nil {
		return err
	}
	ts := append(append([]byte{}, sscBytes...), encSSC...)

	raw, err := u.call(ctx, gAPI, false, []map[string]string{{
		"a":  "up",
		"k":  base64URLEncode(encMaster),
		"ts": base64URLEncode(ts),
	}})
	if err != nil {
		return err
	}
	handle, err := parseUserHandle(raw)
	if err != nil {
		return fmt.Errorf("creating upload account: %w", err)
	}
	u.log("ephemeral user %s", handle)

	raw, err = u.call(ctx, gAPI, false, []map[string]string{{
		"a":    "us",
		"user": handle,
	}})
	if err != nil {
		return err
	}
	session, err := ensureOK(raw)
	if err != nil {
		return fmt.Errorf("starting session: %w", err)
	}
	var us struct {
		TSID string `json:"tsid"`
	}
	if err := json.Unmarshal(session, &us); err != nil || us.TSID == "" {
		return fmt.Errorf("session response missing tsid: %s", truncate(string(session), 180))
	}
	if err := verifySession(masterBytes, us.TSID); err != nil {
		return err
	}
	u.sid = us.TSID

	if _, err := u.call(ctx, gAPI, false, []map[string]string{{"a": "ug"}}); err != nil {
		return fmt.Errorf("loading account: %w", err)
	}
	return nil
}

func parseUserHandle(raw json.RawMessage) (string, error) {
	body, err := ensureOK(raw)
	if err != nil {
		return "", err
	}
	var found []string
	collectStrings(body, &found)
	collectStrings(raw, &found)
	for _, s := range found {
		if len(s) == 11 {
			return s, nil
		}
	}
	return "", fmt.Errorf("unexpected account response: %s", truncate(string(raw), 180))
}

func verifySession(masterKey []byte, tsid string) error {
	raw, err := base64URLDecode(tsid)
	if err != nil {
		return fmt.Errorf("decoding session: %w", err)
	}
	if len(raw) < 32 {
		return fmt.Errorf("session token is too short")
	}
	got, err := aesEncryptBlock(masterKey, raw[:16])
	if err != nil {
		return err
	}
	if !bytes.Equal(got, raw[len(raw)-16:]) {
		return fmt.Errorf("session challenge did not match")
	}
	return nil
}

func (u *Uploader) createTransfer(ctx context.Context, name string) (xh, root string, err error) {
	key, err := randomA32(4)
	if err != nil {
		return "", "", err
	}
	at, err := encryptAttr(folderAttrJSON(name, time.Now().Unix()), key)
	if err != nil {
		return "", "", err
	}
	raw, err := u.call(ctx, bt7API, true, []map[string]string{{
		"a":  "xn",
		"at": at,
		"k":  base64URLEncode(a32ToBytes(key)),
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
		return "", "", fmt.Errorf("unexpected transfer response: %s", truncate(string(raw), 180))
	}
	return xh, root, nil
}

func (u *Uploader) finishTransfer(ctx context.Context, xh, title, password string, expiry int) error {
	req := map[string]any{
		"a":  "xm",
		"xh": xh,
		"t":  base64URLEncode([]byte(title)),
		"e":  expiry,
	}
	if password != "" {
		req["pw"] = derivePassword(xh, password)
	}
	raw, err := u.call(ctx, bt7API, true, []map[string]any{req})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("setting transfer options: %w", err)
	}
	raw, err = u.call(ctx, bt7API, true, []map[string]string{{"a": "xc", "xh": xh}})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("closing transfer: %w", err)
	}
	return nil
}

func (u *Uploader) deleteTransfer(ctx context.Context, xh string) {
	_, _ = u.call(ctx, bt7API, true, []map[string]string{{"a": "xd", "xh": xh}})
}

type uploadPool struct {
	host  string
	uri   string
	limit int64
}

func (u *Uploader) uploadPools(ctx context.Context) ([]uploadPool, error) {
	raw, err := u.call(ctx, gAPI, false, []map[string]string{{"a": "usc"}})
	if err != nil {
		return nil, err
	}
	body, err := ensureOK(raw)
	if err != nil {
		return nil, fmt.Errorf("upload servers: %w", err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("upload servers: %s", truncate(string(body), 180))
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
			return nil, fmt.Errorf("upload server entry: %s", truncate(string(row), 120))
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

func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
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

type uploadMsg struct {
	fileno uint32
	pos    uint64
	typ    int8
	token  []byte
}

func parseUploadMsg(data []byte) (uploadMsg, error) {
	if len(data) < 17 {
		return uploadMsg{}, fmt.Errorf("short upload reply (%d bytes)", len(data))
	}
	body := data[:len(data)-4]
	want := binary.LittleEndian.Uint32(data[len(data)-4:])
	if crc32.ChecksumIEEE(body) != want {
		return uploadMsg{}, fmt.Errorf("upload reply checksum mismatch")
	}
	msg := uploadMsg{
		fileno: binary.LittleEndian.Uint32(body[0:4]),
		pos:    binary.LittleEndian.Uint64(body[4:12]),
		typ:    int8(body[12]),
	}
	if msg.typ == 4 {
		if len(body) < 14 {
			return uploadMsg{}, fmt.Errorf("short completion reply")
		}
		n := int(body[13])
		if len(body) < 14+n {
			return uploadMsg{}, fmt.Errorf("completion token truncated")
		}
		msg.token = append([]byte(nil), body[14:14+n]...)
	}
	return msg, nil
}

func completionHandle(token []byte) (string, error) {
	if len(token) == 36 {
		if token[35] == 1 {
			return base64URLEncode(token), nil
		}
		return strings.TrimRight(string(token), "\x00"), nil
	}
	if len(token) > 0 && isASCIITicket(token) {
		return strings.TrimRight(string(token), "\x00"), nil
	}
	return "", fmt.Errorf("unexpected upload ticket (%d bytes)", len(token))
}

func isASCIITicket(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func chunkHeader(fileno uint32, pos int64, chunk []byte) []byte {
	header := make([]byte, 20)
	binary.LittleEndian.PutUint32(header[0:4], fileno)
	binary.LittleEndian.PutUint64(header[4:12], uint64(pos))
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(chunk)))
	sum := crc32.Update(0, crc32.IEEETable, header[:16])
	sum = crc32.Update(sum, crc32.IEEETable, chunk)
	binary.LittleEndian.PutUint32(header[16:20], sum)
	return header
}

func (u *Uploader) readUploadMsg(ctx context.Context, conn *websocket.Conn) (uploadMsg, error) {
	rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_, data, err := conn.Read(rctx)
	if err != nil {
		return uploadMsg{}, err
	}
	msg, err := parseUploadMsg(data)
	if err != nil {
		u.log("bad upload frame (%d bytes): %v", len(data), err)
		return uploadMsg{}, err
	}
	u.log("upload reply type=%d pos=%d fileno=%d token=%d", msg.typ, msg.pos, msg.fileno, len(msg.token))
	return msg, nil
}

func (u *Uploader) sendChunk(ctx context.Context, conn *websocket.Conn, fileno uint32, pos int64, chunk []byte) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		wctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err := conn.Write(wctx, websocket.MessageBinary, chunkHeader(fileno, pos, chunk))
		if err == nil {
			payload := chunk
			if payload == nil {
				payload = []byte{}
			}
			err = conn.Write(wctx, websocket.MessageBinary, payload)
		}
		cancel()
		if err != nil {
			return nil, err
		}
		for {
			msg, err := u.readUploadMsg(ctx, conn)
			if err != nil {
				return nil, err
			}
			if msg.fileno != fileno && msg.fileno != 0 {
				continue
			}
			if msg.typ < 0 {
				return nil, fmt.Errorf("upload server rejected chunk at %d (%d)", pos, msg.typ)
			}
			switch msg.typ {
			case 3:
				last = fmt.Errorf("upload server asked to resend chunk at %d", pos)
				goto retry
			case 4:
				return msg.token, nil
			case 5:
				return nil, fmt.Errorf("upload server is busy")
			case 1, 2, 7:
				if msg.pos == uint64(pos) {
					return nil, nil
				}
			}
		}
	retry:
	}
	if last == nil {
		last = fmt.Errorf("upload chunk at %d failed", pos)
	}
	return nil, last
}

func (u *Uploader) readTicket(ctx context.Context, conn *websocket.Conn, fileno uint32) ([]byte, error) {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		msg, err := u.readUploadMsg(ctx, conn)
		if err != nil {
			return nil, err
		}
		if msg.typ == 4 && (msg.fileno == fileno || msg.fileno == 0) {
			return msg.token, nil
		}
		if msg.typ < 0 {
			return nil, fmt.Errorf("upload failed (%d)", msg.typ)
		}
	}
	return nil, fmt.Errorf("timed out waiting for upload confirmation")
}

func (u *Uploader) putFile(ctx context.Context, root, path, name string, onBytes func(int64)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", path)
	}
	size := info.Size()

	pools, err := u.uploadPools(ctx)
	if err != nil {
		return err
	}
	pool, err := choosePool(pools, size)
	if err != nil {
		return err
	}
	wsURL := "wss://" + pool.host + "/" + pool.uri
	u.log("upload %s (%d bytes) via %s", name, size, pool.host)

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin":     []string{transferIT},
			"User-Agent": []string{userAgent},
		},
		CompressionMode: websocket.CompressionDisabled,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("connecting to upload server: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(8 << 20)

	ulKey, err := randomA32(6)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(a32ToBytes(ulKey[:4]))
	if err != nil {
		return err
	}
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint32(nonce[:4], ulKey[4])
	binary.BigEndian.PutUint32(nonce[4:], ulKey[5])

	parts, trailingEmpty := fileChunks(size)
	var counter uint64
	var macs [][]byte
	var token []byte
	const fileno uint32 = 1

	for _, part := range parts {
		plain := make([]byte, part[1])
		if _, err := io.ReadFull(f, plain); err != nil {
			return err
		}
		ciphertext, mac := encryptChunk(block, nonce, &counter, plain)
		macs = append(macs, mac)
		tok, err := u.sendChunk(ctx, conn, fileno, part[0], ciphertext)
		if err != nil {
			return err
		}
		if len(tok) > 0 {
			token = tok
		}
		if onBytes != nil {
			onBytes(part[1])
		}
	}
	if trailingEmpty && len(token) == 0 {
		tok, err := u.sendChunk(ctx, conn, fileno, size, nil)
		if err != nil {
			return err
		}
		if len(tok) > 0 {
			token = tok
		}
	}
	if len(token) == 0 {
		token, err = u.readTicket(ctx, conn, fileno)
		if err != nil {
			return err
		}
	}
	handle, err := completionHandle(token)
	if err != nil {
		return err
	}

	fileKey := buildFileKey(ulKey, condenseMacs(block, macs))
	attrs, err := encryptAttr(fileAttrJSON(name), ulKey[:4])
	if err != nil {
		return err
	}
	raw, err := u.call(ctx, bt7API, true, []map[string]any{{
		"a": "xp",
		"v": 3,
		"t": root,
		"n": []map[string]any{{
			"t": 0,
			"h": handle,
			"a": attrs,
			"k": base64URLEncode(a32ToBytes(fileKey)),
		}},
	}})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("saving %s: %w", name, err)
	}
	return nil
}

func naturalChunkLen(pos int64) int64 {
	var p, dp int64
	for dp < 1<<20 {
		dp += 1 << 17
		if p == pos {
			return dp
		}
		p += dp
	}
	return 1 << 20
}

func fileChunks(size int64) (parts [][2]int64, trailingEmpty bool) {
	var pos int64
	for pos < size {
		natural := naturalChunkLen(pos)
		n := natural
		if pos+n > size {
			n = size - pos
		}
		parts = append(parts, [2]int64{pos, n})
		pos += n
		if n < natural {
			return parts, false
		}
	}
	return parts, true
}

func (u *Uploader) Run(ctx context.Context, items []uploadItem, opt uploadOptions) (*uploadResult, error) {
	if err := u.createSession(ctx); err != nil {
		return nil, err
	}
	xh, root, err := u.createTransfer(ctx, opt.title)
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			u.deleteTransfer(context.Background(), xh)
		}
	}()

	if u.pretty {
		draw(layoutStatus(contentWidth(), "Upload", "Opening transfer"))
	}

	var total int64
	for _, item := range items {
		info, err := os.Stat(item.path)
		if err != nil {
			return nil, err
		}
		total += info.Size()
	}
	var sent int64
	start := time.Now()
	for _, item := range items {
		info, err := os.Stat(item.path)
		if err != nil {
			return nil, err
		}
		fileStart := sent
		if !u.silent {
			u.showProgress(item.name, sent, total, start)
		}
		err = u.putFile(ctx, root, item.path, item.name, func(n int64) {
			sent += n
			if !u.silent {
				u.showProgress(item.name, sent, total, start)
			}
		})
		if err != nil {
			if !u.silent && !u.pretty {
				fmt.Fprintln(os.Stderr)
			}
			return nil, err
		}
		if sent < fileStart+info.Size() {
			sent = fileStart + info.Size()
		}
		if !u.silent {
			u.showProgress(item.name, sent, total, start)
			if !u.pretty {
				fmt.Fprintln(os.Stderr)
			}
		}
	}

	if err := u.finishTransfer(ctx, xh, opt.title, opt.password, opt.expiry); err != nil {
		return nil, err
	}
	closed = true
	return &uploadResult{
		link:     transferIT + "/t/" + xh,
		password: opt.password,
		expiry:   opt.expiry,
		title:    opt.title,
		files:    len(items),
		bytes:    total,
	}, nil
}

func (u *Uploader) showProgress(name string, done, total int64, start time.Time) {
	if u.pretty {
		draw(layoutProgress(contentWidth(), "Upload", name, done, total, start))
		return
	}
	printTransferProgress(name, done, total, start)
}

func prepareItems(paths []string) ([]uploadItem, error) {
	var items []uploadItem
	seen := map[string]int{}
	unique := func(name string) string {
		if seen[name] == 0 {
			seen[name]++
			return name
		}
		seen[name]++
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		return fmt.Sprintf("%s (%d)%s", base, seen[name]-1, ext)
	}
	cleanup := func() {
		for _, item := range items {
			if item.cleanup != nil {
				item.cleanup()
			}
		}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			cleanup()
			return nil, err
		}
		if info.IsDir() {
			zipPath, zipName, err := zipDirectory(path)
			if err != nil {
				cleanup()
				return nil, err
			}
			items = append(items, uploadItem{
				path:    zipPath,
				name:    unique(zipName),
				title:   filepath.Base(path),
				cleanup: func() { os.Remove(zipPath) },
			})
			continue
		}
		if !info.Mode().IsRegular() {
			cleanup()
			return nil, fmt.Errorf("%s is not a file or folder", path)
		}
		name := filepath.Base(path)
		items = append(items, uploadItem{path: path, name: unique(name), title: name})
	}
	return items, nil
}

func zipDirectory(dir string) (string, string, error) {
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "folder"
	}
	f, err := os.CreateTemp("", "transferit-*.zip")
	if err != nil {
		return "", "", err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeZip := zw.Close()
	closeFile := f.Close()
	if err != nil {
		return "", "", err
	}
	if closeZip != nil {
		return "", "", closeZip
	}
	if closeFile != nil {
		return "", "", closeFile
	}
	ok = true
	return f.Name(), base + ".zip", nil
}

func runUpload(args []string) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	password := fs.String("p", "", "password override: none, random, or a password (default: saved setting)")
	expiry := fs.String("e", "", "expiry override, for example 1d, 7d, 30d, 90d, or 12h (maximum 90 days)")
	title := fs.String("t", "", "transfer title")
	verbose := fs.Bool("v", false, "show API requests")
	silent := fs.Bool("s", false, "print only the link and password")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transferit upload [options] <file or folder>...\n\n")
		fmt.Fprintf(os.Stderr, "Folders are zipped and uploaded as a single .zip file.\n")
		fmt.Fprintf(os.Stderr, "Password and expiry come from 'transferit config' unless overridden.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  transferit up report.pdf\n")
		fmt.Fprintf(os.Stderr, "  transferit up -p random -e 7d ./photos\n")
		fmt.Fprintf(os.Stderr, "  transferit upload -p none -e 1d -t Notes a.txt b.txt\n")
	}
	fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	mode, pw := cfg.PasswordMode, cfg.Password
	expStr := cfg.Expiry
	passwordSet, expirySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "p":
			passwordSet = true
		case "e":
			expirySet = true
		}
	})
	if passwordSet {
		mode, pw = parsePasswordChoice(*password)
	}
	if expirySet {
		expStr = *expiry
	}
	seconds, err := parseExpiry(expStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := validateExpiry(seconds); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	switch mode {
	case "random":
		pw, err = randomPassword(16)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "fixed":
		pw = strings.TrimSpace(pw)
	default:
		pw = ""
	}

	pretty := false
	if !*silent && stdinIsTerminal() {
		if err := enterUI(); err == nil {
			pretty = true
		}
	}
	defer exitUI()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	setCtrlC(func() { stop() })

	if pretty {
		draw(layoutStatus(contentWidth(), "Upload", "Preparing files"))
	} else if !*silent {
		fmt.Fprintf(os.Stderr, "Preparing files...\n")
	}
	items, err := prepareItems(fs.Args())
	if err != nil {
		exitUI()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, item := range items {
			if item.cleanup != nil {
				item.cleanup()
			}
		}
	}()

	transferTitle := strings.TrimSpace(*title)
	if transferTitle == "" {
		if len(items) == 1 {
			transferTitle = items[0].title
		} else {
			transferTitle = "Transfer"
		}
	}

	up := NewUploader(*verbose, *silent)
	up.pretty = pretty
	result, err := up.Run(ctx, items, uploadOptions{
		title:    transferTitle,
		password: pw,
		expiry:   seconds,
	})
	exitUI()
	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Upload aborted.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if *silent {
		fmt.Println(result.link)
		if result.password != "" {
			fmt.Println(result.password)
		}
		return
	}
	fmt.Printf("\n  uploaded  %d %s (%s)\n\n", result.files, fileWord(result.files), formatSize(result.bytes))
	fmt.Printf("  link      %s\n", result.link)
	fmt.Printf("  title     %s\n", result.title)
	if result.password != "" {
		fmt.Printf("  password  %s\n", result.password)
	} else {
		fmt.Printf("  password  none\n")
	}
	fmt.Printf("  expires   %s\n", expiryPhrase(result.expiry))
}

func fileWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
