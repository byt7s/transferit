package upload

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"transferit/internal/api"
	"transferit/internal/crypto"
	"transferit/internal/retry"
)

const (
	uploadConns  = 8
	uploadWindow = 1500000
	ackTimeout   = 30 * time.Second
	writeLimit   = 3 * time.Minute
)

var errStop = errors.New("upload stopped")

type piece struct {
	pos int64
	n   int64
}

type sentPiece struct {
	piece
	at time.Time
}

type pipe struct {
	mu    sync.Mutex
	queue []piece
	acked map[int64]struct{}
	macs  map[int64][]byte
	token []byte
	fatal error
	dataN int
}

func newPipe(jobs []piece, dataN int) *pipe {
	return &pipe{
		queue: append([]piece(nil), jobs...),
		acked: map[int64]struct{}{},
		macs:  map[int64][]byte{},
		dataN: dataN,
	}
}

func (p *pipe) take() (piece, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.token) > 0 || p.fatal != nil || len(p.queue) == 0 {
		return piece{}, false
	}
	job := p.queue[0]
	p.queue = p.queue[1:]
	return job, true
}

func (p *pipe) requeue(jobs []piece) {
	if len(jobs) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.token) > 0 || p.fatal != nil {
		return
	}
	keep := make([]piece, 0, len(jobs))
	for _, job := range jobs {
		if _, ok := p.acked[job.pos]; ok {
			continue
		}
		keep = append(keep, job)
	}
	if len(keep) == 0 {
		return
	}
	p.queue = append(keep, p.queue...)
}

func (p *pipe) ack(pos, n int64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.acked[pos]; ok {
		return 0
	}
	p.acked[pos] = struct{}{}
	if n > 0 {
		return n
	}
	return 0
}

func (p *pipe) setMAC(pos int64, mac []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.macs[pos]; ok {
		return
	}
	p.macs[pos] = append([]byte(nil), mac...)
}

func (p *pipe) complete(token []byte) {
	p.mu.Lock()
	if len(p.token) == 0 {
		p.token = append([]byte(nil), token...)
	}
	p.mu.Unlock()
}

func (p *pipe) hasToken() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.token) > 0
}

func (p *pipe) stop(err error) {
	if err == nil {
		return
	}
	p.mu.Lock()
	if p.fatal == nil {
		p.fatal = err
	}
	p.mu.Unlock()
}

func (p *pipe) fatalErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fatal
}

func (p *pipe) result() ([]byte, [][]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.token) > 0 {
		if len(p.macs) != p.dataN {
			return nil, nil, fmt.Errorf("upload confirmation missing %d chunk checksums", p.dataN-len(p.macs))
		}
		return append([]byte(nil), p.token...), orderedMACs(p.macs), nil
	}
	if p.fatal != nil {
		return nil, nil, p.fatal
	}
	return nil, nil, fmt.Errorf("upload ended without confirmation")
}

func orderedMACs(macs map[int64][]byte) [][]byte {
	if len(macs) == 0 {
		return nil
	}
	pos := make([]int64, 0, len(macs))
	for at := range macs {
		pos = append(pos, at)
	}
	sort.Slice(pos, func(i, j int) bool { return pos[i] < pos[j] })
	out := make([][]byte, len(pos))
	for i, at := range pos {
		out[i] = macs[at]
	}
	return out
}

func encryptAt(block cipher.Block, nonce []byte, pos int64, plain []byte) ([]byte, []byte) {
	counter := uint64(pos) / aes.BlockSize
	return crypto.EncryptChunk(block, nonce, &counter, plain)
}

type inbox struct {
	mu  sync.Mutex
	q   []uploadMsg
	err error
	ch  chan struct{}
}

func newInbox() *inbox {
	return &inbox{ch: make(chan struct{}, 1)}
}

func (b *inbox) push(msg uploadMsg) {
	b.mu.Lock()
	b.q = append(b.q, msg)
	b.mu.Unlock()
	b.wake()
}

func (b *inbox) fail(err error) {
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
	b.wake()
}

func (b *inbox) wake() {
	select {
	case b.ch <- struct{}{}:
	default:
	}
}

func (b *inbox) pop() (uploadMsg, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.q) == 0 {
		return uploadMsg{}, false
	}
	msg := b.q[0]
	b.q = b.q[1:]
	return msg, true
}

func (b *inbox) readErr() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.q) > 0 {
		return nil
	}
	return b.err
}

func (u *Uploader) sendParallel(parent context.Context, f *os.File, size int64, wsURL string, key, nonce []byte, onBytes func(int64)) ([]byte, [][]byte, error) {
	parts, trailing := fileChunks(size)
	jobs := make([]piece, 0, len(parts)+1)
	for _, part := range parts {
		jobs = append(jobs, piece{pos: part[0], n: part[1]})
	}
	if trailing {
		jobs = append(jobs, piece{pos: size, n: 0})
	}
	if len(jobs) == 0 {
		return nil, nil, fmt.Errorf("empty upload")
	}
	state := newPipe(jobs, len(parts))
	watch := retry.NewWatch()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var prog sync.Mutex
	report := func(n int64) {
		if n <= 0 || onBytes == nil {
			return
		}
		prog.Lock()
		onBytes(n)
		prog.Unlock()
	}

	n := uploadConns
	if n > len(jobs) {
		n = len(jobs)
	}
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u.connWorker(ctx, cancel, state, watch, f, wsURL, key, nonce, report)
		}()
	}
	wg.Wait()
	if tok, macs, err := state.result(); err == nil {
		return tok, macs, nil
	} else if parent.Err() != nil {
		return nil, nil, parent.Err()
	} else {
		return nil, nil, err
	}
}

func (u *Uploader) connWorker(ctx context.Context, cancel context.CancelFunc, state *pipe, watch *retry.Watch, f *os.File, wsURL string, key, nonce []byte, onBytes func(int64)) {
	delay := watch.Step()
	for {
		if state.hasToken() {
			cancel()
			return
		}
		if err := state.fatalErr(); err != nil {
			cancel()
			return
		}
		if ctx.Err() != nil {
			return
		}
		conn, err := dialUpload(ctx, wsURL)
		if err != nil {
			u.giveUp(ctx, cancel, state, watch, &delay, err)
			continue
		}
		err = u.serveConn(ctx, conn, state, watch, f, key, nonce, onBytes)
		_ = conn.Close(websocket.StatusGoingAway, "")
		if state.hasToken() {
			cancel()
			return
		}
		if errors.Is(err, errStop) {
			state.stop(err)
			cancel()
			return
		}
		if ctx.Err() != nil {
			return
		}
		u.giveUp(ctx, cancel, state, watch, &delay, err)
	}
}

func (u *Uploader) giveUp(ctx context.Context, cancel context.CancelFunc, state *pipe, watch *retry.Watch, delay *time.Duration, cause error) {
	if state.hasToken() || ctx.Err() != nil {
		return
	}
	if err := u.waitNet(ctx, watch, delay); err != nil {
		if ctx.Err() == nil {
			state.stop(err)
		}
		cancel()
	} else if cause != nil {
		u.log("upload connection retry: %v", cause)
	}
}

func (u *Uploader) waitNet(ctx context.Context, watch *retry.Watch, delay *time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := watch.Arm(); err != nil {
		return err
	}
	watch.Tell(func() {
		u.showStatus("Upload", retry.WaitNotice)
	})
	err := retry.Sleep(ctx, *delay)
	if *delay < watch.Cap() {
		*delay *= 2
		if *delay > watch.Cap() {
			*delay = watch.Cap()
		}
	}
	return err
}

func dialUpload(ctx context.Context, wsURL string) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin":     []string{api.Web},
			"User-Agent": []string{api.UserAgent},
		},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(8 << 20)
	return conn, nil
}

func (u *Uploader) serveConn(ctx context.Context, conn *websocket.Conn, state *pipe, watch *retry.Watch, f *os.File, key, nonce []byte, onBytes func(int64)) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("%w: %v", errStop, err)
	}
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	box := newInbox()
	go func() {
		for {
			msg, err := readUploadFrame(connCtx, conn)
			if err != nil {
				box.fail(err)
				return
			}
			u.log("upload reply type=%d pos=%d fileno=%d token=%d", msg.typ, msg.pos, msg.fileno, len(msg.token))
			box.push(msg)
		}
	}()

	var sent []sentPiece
	var inFlight int64
	defer func() {
		jobs := make([]piece, len(sent))
		for i, item := range sent {
			jobs[i] = item.piece
		}
		state.requeue(jobs)
	}()

	const fileno uint32 = 1
	for {
		if state.hasToken() {
			return nil
		}
		if err := state.fatalErr(); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for {
			msg, ok := box.pop()
			if !ok {
				break
			}
			if err := handleUploadMsg(msg, state, watch, &sent, &inFlight, fileno, onBytes); err != nil {
				return err
			}
			if state.hasToken() {
				return nil
			}
		}
		if err := box.readErr(); err != nil {
			return err
		}
		if len(sent) > 0 && time.Since(sent[0].at) > ackTimeout {
			return fmt.Errorf("chunk acknowledgement timed out")
		}

		for inFlight < uploadWindow {
			if state.hasToken() {
				return nil
			}
			job, ok := state.take()
			if !ok {
				break
			}
			var payload []byte
			if job.n > 0 {
				plain := make([]byte, job.n)
				if _, err := f.ReadAt(plain, job.pos); err != nil {
					sent = append(sent, sentPiece{piece: job, at: time.Now()})
					return fmt.Errorf("%w: reading file: %v", errStop, err)
				}
				ciphertext, mac := encryptAt(block, nonce, job.pos, plain)
				state.setMAC(job.pos, mac)
				payload = ciphertext
			}
			sent = append(sent, sentPiece{piece: job, at: time.Now()})
			inFlight += job.n
			if err := writeChunk(ctx, conn, fileno, job.pos, payload); err != nil {
				return err
			}
		}

		wait := 500 * time.Millisecond
		if len(sent) > 0 {
			wait = ackTimeout - time.Since(sent[0].at)
			if wait < 0 {
				return fmt.Errorf("chunk acknowledgement timed out")
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-box.ch:
			timer.Stop()
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			if state.hasToken() {
				return nil
			}
			return ctx.Err()
		}
	}
}

func handleUploadMsg(msg uploadMsg, state *pipe, watch *retry.Watch, sent *[]sentPiece, inFlight *int64, fileno uint32, onBytes func(int64)) error {
	if msg.fileno != fileno && msg.fileno != 0 {
		return nil
	}
	if msg.typ < 0 {
		err := fmt.Errorf("%w: upload rejected (%d)", errStop, msg.typ)
		state.stop(err)
		return err
	}
	switch msg.typ {
	case 5:
		return fmt.Errorf("upload server asked to reconnect")
	case 3:
		if n, ok := takeSent(sent, int64(msg.pos)); ok {
			*inFlight -= n
			state.requeue([]piece{{pos: int64(msg.pos), n: n}})
		}
	case 1, 2, 4, 7:
		if n, ok := takeSent(sent, int64(msg.pos)); ok {
			*inFlight -= n
			if got := state.ack(int64(msg.pos), n); got > 0 && onBytes != nil {
				onBytes(got)
			}
			watch.Mark()
		}
		if msg.typ == 4 && len(msg.token) > 0 {
			state.complete(msg.token)
		}
	}
	return nil
}

func takeSent(sent *[]sentPiece, pos int64) (int64, bool) {
	items := *sent
	for i := range items {
		if items[i].pos == pos {
			n := items[i].n
			*sent = append(items[:i], items[i+1:]...)
			return n, true
		}
	}
	return 0, false
}

func readUploadFrame(ctx context.Context, conn *websocket.Conn) (uploadMsg, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return uploadMsg{}, err
	}
	return parseUploadMsg(data)
}

func writeChunk(ctx context.Context, conn *websocket.Conn, fileno uint32, pos int64, payload []byte) error {
	wctx, cancel := context.WithTimeout(ctx, writeLimit)
	defer cancel()
	if err := conn.Write(wctx, websocket.MessageBinary, chunkHeader(fileno, pos, payload)); err != nil {
		return err
	}
	body := payload
	if body == nil {
		body = []byte{}
	}
	return conn.Write(wctx, websocket.MessageBinary, body)
}
