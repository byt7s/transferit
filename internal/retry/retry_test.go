package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatchBudgetAndReset(t *testing.T) {
	w := &Watch{Budget: 40 * time.Millisecond}
	if err := w.Arm(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := w.Arm(); err == nil {
		t.Fatal("expected the outage budget to end the wait")
	}
	w.Mark()
	if err := w.Arm(); err != nil {
		t.Fatal(err)
	}
}

func TestTellOnceUntilProgress(t *testing.T) {
	w := NewWatch()
	n := 0
	w.Tell(func() { n++ })
	w.Tell(func() { n++ })
	if n != 1 {
		t.Fatalf("told %d", n)
	}
	w.Mark()
	w.Tell(func() { n++ })
	if n != 2 {
		t.Fatalf("told %d", n)
	}
}

func TestSleepReturnsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestStatusTemporary(t *testing.T) {
	if !IsTemporary(StatusError{Code: 503}) {
		t.Fatal("503 should retry")
	}
	if IsTemporary(StatusError{Code: 404}) {
		t.Fatal("404 should not retry")
	}
	if IsTemporary(context.Canceled) {
		t.Fatal("cancel should not retry")
	}
}
