# Simple Circuit Breaker in Go

A small, thread-safe circuit breaker using only the Go standard library.

States: closed -> open (after N consecutive failures) -> half-open (after timeout) -> closed (trial calls succeed) or open (a trial fails).

## Layout

- `breaker/breaker.go` - the circuit breaker
- `breaker/breaker_test.go` - unit tests (fake clock, no sleeping)
- `main.go` - demo calling a local flaky server, with a fallback

## Run

```sh
go test ./...
go run .
```
The demo server returns 503 until call 8, then recovers. Each line shows the
breaker state before and after the call. Expected demo output:

- calls 0-2 fail and the breaker opens
- calls 3-5 fail fast with "circuit breaker is open"
- call 6 is a half-open trial; the server is still down, so it reopens
- calls 7-9 fail fast again (the server recovers at call 8)
- calls 10-11 are two successful trials and the breaker closes
- calls 12-14 succeed normally

## Parameters

```sh
breaker.New(maxFailures, openTimeout, halfOpenMax)
```

Requires Go 1.27 or newer (adjust the `go` line in go.mod for older versions).
