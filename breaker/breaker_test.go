package breaker

import (
	"errors"
	"testing"
	"time"
)

func TestBreakerLifecycle(t *testing.T) {
	cb := New(2, time.Minute, 1)
	clock := time.Now()
	cb.now = func() time.Time { return clock }

	fail := func() error { return errors.New("boom") }
	ok := func() error { return nil }

	cb.Execute(fail)
	cb.Execute(fail)
	if cb.State() != Open {
		t.Fatal("expected open")
	}
	if err := cb.Execute(ok); err != ErrOpen {
		t.Fatalf("expected ErrOpen, got %v", err)
	}

	clock = clock.Add(2 * time.Minute) // time passes
	if err := cb.Execute(ok); err != nil {
		t.Fatal(err)
	}
	if cb.State() != Closed {
		t.Fatal("expected closed after successful trial")
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	cb := New(1, time.Minute, 1)
	clock := time.Now()
	cb.now = func() time.Time { return clock }

	fail := func() error { return errors.New("boom") }

	cb.Execute(fail)
	if cb.State() != Open {
		t.Fatal("expected open")
	}

	clock = clock.Add(2 * time.Minute)
	cb.Execute(fail) // trial call fails
	if cb.State() != Open {
		t.Fatal("expected breaker to reopen after failed trial")
	}
}

func TestHalfOpenLimitsTotalTrials(t *testing.T) {
	cb := New(1, time.Minute, 2)
	clock := time.Now()
	cb.now = func() time.Time { return clock }

	cb.Execute(func() error { return errors.New("boom") })
	clock = clock.Add(2 * time.Minute)

	entered := make(chan struct{})
	release := make(chan struct{})
	slow := make(chan error)
	go func() {
		slow <- cb.Execute(func() error {
			close(entered)
			<-release
			return errors.New("slow trial failed")
		})
	}()
	<-entered

	if err := cb.Execute(func() error { return nil }); err != nil {
		t.Fatalf("second trial: %v", err)
	}
	if err := cb.Execute(func() error { return nil }); err != ErrTooManyRequests {
		t.Fatalf("third trial: expected ErrTooManyRequests, got %v", err)
	}

	close(release)
	<-slow
	if cb.State() != Open {
		t.Fatalf("expected open after a failed trial, got %s", cb.State())
	}
}

func TestPanickingTrialReopens(t *testing.T) {
	cb := New(1, time.Minute, 1)
	clock := time.Now()
	cb.now = func() time.Time { return clock }

	cb.Execute(func() error { return errors.New("boom") })
	clock = clock.Add(2 * time.Minute)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic to propagate")
			}
		}()
		cb.Execute(func() error { panic("trial panicked") })
	}()

	if cb.State() != Open {
		t.Fatalf("expected open after a panicking trial, got %s", cb.State())
	}
	if err := cb.Execute(func() error { return nil }); err != ErrOpen {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
}

func TestStateReportsHalfOpenAfterTimeout(t *testing.T) {
	cb := New(1, time.Minute, 1)
	clock := time.Now()
	cb.now = func() time.Time { return clock }

	cb.Execute(func() error { return errors.New("boom") })
	clock = clock.Add(2 * time.Minute)

	if cb.State() != HalfOpen {
		t.Fatalf("expected half-open after timeout, got %s", cb.State())
	}
}
