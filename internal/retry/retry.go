// Package retry waits out a dropped connection and tries again.
package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const WaitNotice = "Connection lost. Resuming when the network is back."

// Watch is the clock for one transfer attempt. Progress clears it.
// A stretch of failures longer than Budget ends the wait.
type Watch struct {
	Budget   time.Duration
	MaxDelay time.Duration
	Floor    time.Duration

	mu   sync.Mutex
	down time.Time
	told bool
}

func NewWatch() *Watch {
	return &Watch{
		Budget:   12 * time.Minute,
		MaxDelay: 15 * time.Second,
		Floor:    time.Second,
	}
}

func (w *Watch) Mark() {
	w.mu.Lock()
	w.down = time.Time{}
	w.told = false
	w.mu.Unlock()
}

func (w *Watch) Arm() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	budget := w.Budget
	if budget <= 0 {
		budget = 12 * time.Minute
	}
	if w.down.IsZero() {
		w.down = time.Now()
	}
	if time.Since(w.down) >= budget {
		return fmt.Errorf("network unavailable for %s", budget.Truncate(time.Second))
	}
	return nil
}

// Tell runs fn once per outage. Mark arms it again.
func (w *Watch) Tell(fn func()) {
	if fn == nil {
		return
	}
	w.mu.Lock()
	if w.told {
		w.mu.Unlock()
		return
	}
	w.told = true
	w.mu.Unlock()
	fn()
}

func (w *Watch) Step() time.Duration {
	if w.Floor > 0 {
		return w.Floor
	}
	return time.Second
}

func (w *Watch) Cap() time.Duration {
	if w.MaxDelay > 0 {
		return w.MaxDelay
	}
	return 15 * time.Second
}

func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type StatusError struct {
	Code int
	Body string
}

func (e StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("HTTP %d", e.Code)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Body)
}

func IsTemporary(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var status StatusError
	if errors.As(err, &status) {
		return status.Code == 408 || status.Code == 429 || status.Code >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var timed interface{ Timeout() bool }
	if errors.As(err, &timed) && timed.Timeout() {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func NewTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	transport := base.Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxIdleConnsPerHost = 8
	return transport
}
