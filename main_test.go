package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestGiftHandlerConcurrentRequests(t *testing.T) {
	balancesMu.Lock()
	balances = map[int]int{1: 100}
	balancesMu.Unlock()

	const requests = 20

	type result struct {
		statusCode int
		errorText  string
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan result, requests)

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-start

			body := bytes.NewBufferString(`{"userId":1,"streamerId":100,"amount":10}`)
			req := httptest.NewRequest(http.MethodPost, "/gift", body)
			rec := httptest.NewRecorder()

			giftHandler(rec, req)

			res := result{statusCode: rec.Code}
			if rec.Code == http.StatusBadRequest {
				var body errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&body); err == nil {
					res.errorText = body.Error
				}
			}

			results <- res
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	insufficientBalanceFailures := 0

	for res := range results {
		switch res.statusCode {
		case http.StatusOK:
			successes++
		case http.StatusBadRequest:
			if res.errorText != "insufficient balance" {
				t.Fatalf("expected insufficient balance error, got %q", res.errorText)
			}
			insufficientBalanceFailures++
		default:
			t.Fatalf("unexpected status code: %d", res.statusCode)
		}
	}

	if successes != 10 {
		t.Fatalf("expected 10 successful requests, got %d", successes)
	}

	if insufficientBalanceFailures != 10 {
		t.Fatalf("expected 10 insufficient balance failures, got %d", insufficientBalanceFailures)
	}

	balancesMu.Lock()
	finalBalance := balances[1]
	balancesMu.Unlock()

	t.Logf("20 concurrent gifts: successes=%d, insufficient_balance_failures=%d, final_balance=%d", successes, insufficientBalanceFailures, finalBalance)

	if finalBalance != 0 {
		t.Fatalf("expected final balance to be 0, got %d", finalBalance)
	}
}

func TestGiftHandlerTwoConcurrentLargeGifts(t *testing.T) {
	balancesMu.Lock()
	balances = map[int]int{1: 100}
	balancesMu.Unlock()

	const requests = 2

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan int, requests)

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-start

			body := bytes.NewBufferString(`{"userId":1,"streamerId":100,"amount":80}`)
			req := httptest.NewRequest(http.MethodPost, "/gift", body)
			rec := httptest.NewRecorder()

			giftHandler(rec, req)
			results <- rec.Code
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	failures := 0

	for status := range results {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusBadRequest:
			failures++
		default:
			t.Fatalf("unexpected status code: %d", status)
		}
	}

	if successes != 1 {
		t.Fatalf("expected 1 successful request, got %d", successes)
	}

	if failures != 1 {
		t.Fatalf("expected 1 insufficient balance failure, got %d", failures)
	}

	balancesMu.Lock()
	finalBalance := balances[1]
	balancesMu.Unlock()

	t.Logf("two concurrent 80-coin gifts: successes=%d, failures=%d, final_balance=%d", successes, failures, finalBalance)

	if finalBalance != 20 {
		t.Fatalf("expected final balance to be 20, got %d", finalBalance)
	}
}

func TestGiftHandlerCanceledRequestDoesNotSpendBalance(t *testing.T) {
	balancesMu.Lock()
	balances = map[int]int{1: 100}
	balancesMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	body := bytes.NewBufferString(`{"userId":1,"streamerId":100,"amount":10}`)
	req := httptest.NewRequest(http.MethodPost, "/gift", body).WithContext(ctx)
	rec := httptest.NewRecorder()

	giftHandler(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("expected status %d, got %d", http.StatusRequestTimeout, rec.Code)
	}

	balancesMu.Lock()
	finalBalance := balances[1]
	balancesMu.Unlock()

	t.Logf("canceled gift request: status=%d, final_balance=%d", rec.Code, finalBalance)

	if finalBalance != 100 {
		t.Fatalf("expected canceled request to leave balance at 100, got %d", finalBalance)
	}
}
