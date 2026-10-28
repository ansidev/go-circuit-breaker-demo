package breaker

import (
	"errors"
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	default:
		return "half-open"
	}
}

var (
	ErrOpen            = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("circuit breaker is half-open: too many trial requests")

	errPanicked = errors.New("circuit breaker: call panicked")
)

type Breaker struct {
	mu         sync.Mutex
	state      State
	generation uint64 // bumps on every state change
	failures   int    // consecutive failures (closed)
	successes  int    // successful trials (half-open)
	trials     int    // trials admitted in this half-open period, finished or not
	openedAt   time.Time

	maxFailures int           // failures before tripping
	openTimeout time.Duration // how long to stay open
	halfOpenMax int           // trial calls allowed in half-open

	now func() time.Time // injectable for tests
}

func New(maxFailures int, openTimeout time.Duration, halfOpenMax int) *Breaker {
	return &Breaker{
		maxFailures: maxFailures,
		openTimeout: openTimeout,
		halfOpenMax: halfOpenMax,
		now:         time.Now,
	}
}

// Execute runs fn if the breaker allows it and records the result.
// A panic in fn is recorded as a failure and then propagates to the caller.
func (b *Breaker) Execute(fn func() error) error {
	gen, err := b.before()
	if err != nil {
		return err // fail fast, fn is never called
	}
	panicked := true
	defer func() {
		if panicked {
			b.after(gen, errPanicked)
		}
	}()
	err = fn()
	panicked = false
	b.after(gen, err)
	return err
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireOpen()
	return b.state
}

func (b *Breaker) before() (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.expireOpen()
	switch b.state {
	case Open:
		return 0, ErrOpen
	case HalfOpen:
		if b.trials >= b.halfOpenMax {
			return 0, ErrTooManyRequests
		}
		b.trials++
	}
	return b.generation, nil
}

// expireOpen moves an open breaker to half-open once openTimeout has elapsed.
// It must be called with b.mu held.
func (b *Breaker) expireOpen() {
	if b.state == Open && b.now().Sub(b.openedAt) >= b.openTimeout {
		b.setState(HalfOpen)
	}
}

func (b *Breaker) after(gen uint64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// The state changed while this call was running, so its result is stale.
	if gen != b.generation {
		return
	}

	switch b.state {
	case Closed:
		if err == nil {
			b.failures = 0
			return
		}
		b.failures++
		if b.failures >= b.maxFailures {
			b.setState(Open)
		}
	case HalfOpen:
		if err != nil {
			b.setState(Open) // still broken, reopen
			return
		}
		b.successes++
		if b.successes >= b.halfOpenMax {
			b.setState(Closed) // recovered
		}
	}
}

// setState must be called with b.mu held.
func (b *Breaker) setState(s State) {
	b.state = s
	b.generation++
	b.failures, b.successes, b.trials = 0, 0, 0
	if s == Open {
		b.openedAt = b.now()
	}
}
