package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

type termSession struct {
	raw    *term.State
	mu     sync.Mutex
	buf    []byte
	notify chan struct{}
}

var (
	termMu      sync.Mutex
	activeTerm  *termSession
	onCtrlC     = abortDownload
	exitProcess = os.Exit
)

// OnDiscard removes a partial download before the process exits on Ctrl+C.
var OnDiscard func()

func StdinIsTerminal() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stderrIsTerminal() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func Enter() error {
	fd := int(os.Stdin.Fd())
	if !StdinIsTerminal() {
		return fmt.Errorf("no terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	session := &termSession{
		raw:    state,
		notify: make(chan struct{}, 1),
	}
	termMu.Lock()
	activeTerm = session
	termMu.Unlock()
	fmt.Print("\033[?1049h\033[H\033[2J\033[?2004h\033[?25l")
	go session.readLoop()
	return nil
}

func Exit() {
	termMu.Lock()
	session := activeTerm
	activeTerm = nil
	termMu.Unlock()
	if session == nil {
		return
	}
	fmt.Print("\033[?25h\033[?2004l\033[?1049l")
	_ = term.Restore(int(os.Stdin.Fd()), session.raw)
}

func abortDownload() {
	Exit()
	if OnDiscard != nil {
		OnDiscard()
	}
	fmt.Fprint(os.Stdout, "Download aborted.\r\n")
	exitProcess(0)
}

func SetCtrlC(fn func()) {
	termMu.Lock()
	onCtrlC = fn
	termMu.Unlock()
}

func (s *termSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			for _, b := range chunk {
				if b == 3 {
					termMu.Lock()
					fn := onCtrlC
					termMu.Unlock()
					if fn == nil {
						fn = abortDownload
					}
					fn()
					return
				}
			}
			s.mu.Lock()
			s.buf = append(s.buf, chunk...)
			s.mu.Unlock()
			select {
			case s.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *termSession) feed(p []byte) {
	s.mu.Lock()
	s.buf = append(s.buf, p...)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *termSession) tryPop() (byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return 0, false
	}
	b := s.buf[0]
	s.buf = s.buf[1:]
	if len(s.buf) == 0 {
		s.buf = s.buf[:0]
	}
	return b, true
}

func (s *termSession) readByte(timeout time.Duration) (byte, bool) {
	if b, ok := s.tryPop(); ok {
		return b, true
	}
	if timeout == 0 {
		return 0, false
	}
	var timer <-chan time.Time
	if timeout > 0 {
		clock := time.NewTimer(timeout)
		defer clock.Stop()
		timer = clock.C
	}
	for {
		select {
		case <-s.notify:
			if b, ok := s.tryPop(); ok {
				return b, true
			}
		case <-timer:
			if b, ok := s.tryPop(); ok {
				return b, true
			}
			return 0, false
		}
	}
}

func (s *termSession) unread(b byte) {
	s.mu.Lock()
	s.buf = append([]byte{b}, s.buf...)
	s.mu.Unlock()
}
