package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/ansidev/go-circuit-breaker-demo/breaker"
)

var client = &http.Client{Timeout: 2 * time.Second} // always set a timeout

func fetch(cb *breaker.Breaker, url string) (string, error) {
	var body string
	err := cb.Execute(func() error {
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}
		body = resp.Status
		return nil
	})
	return body, err
}

func main() {
	// A local service that returns 503 until it is marked healthy.
	var healthy atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	}))
	defer srv.Close()

	cb := breaker.New(3, 1800*time.Millisecond, 2) // 3 failures, 1.8s open, 2 trials

	for i := 0; i < 15; i++ {
		if i == 8 {
			healthy.Store(true)
			fmt.Println("--- service recovered ---")
		}

		before := cb.State()
		res, err := fetch(cb, srv.URL)
		after := cb.State()

		if err != nil {
			// Fallback: cached or default data
			fmt.Printf("call %2d: %-9s -> %-9s err=%v -> using fallback\n", i, before, after, err)
		} else {
			fmt.Printf("call %2d: %-9s -> %-9s ok: %s\n", i, before, after, res)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
