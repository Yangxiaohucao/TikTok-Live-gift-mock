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

func TestScenario1_20ConcurrentGiftRequests(t *testing.T) {
	// 场景 1：
	// 用户 1 余额是 100。
	// 20 个请求几乎同时发送礼物，每个请求扣 10。
	// 正确结果：10 个成功，10 个余额不足失败，最终余额是 0。
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

	t.Logf("场景 1：20 个并发请求，每个扣 10。成功=%d，余额不足失败=%d，最终余额=%d", successes, insufficientBalanceFailures, finalBalance)

	if finalBalance != 0 {
		t.Fatalf("expected final balance to be 0, got %d", finalBalance)
	}
}

func TestScenario2_TwoConcurrentLargeGifts(t *testing.T) {
	// 场景 2：
	// 用户 1 余额是 100。
	// 两个请求几乎同时发送大额礼物，每个请求扣 80。
	// 正确结果：只能有 1 个成功，另 1 个必须余额不足失败，最终余额是 20。
	// 这个场景专门用来理解 race condition。
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

	t.Logf("场景 2：两个并发大额请求，每个扣 80。成功=%d，余额不足失败=%d，最终余额=%d", successes, failures, finalBalance)

	if finalBalance != 20 {
		t.Fatalf("expected final balance to be 20, got %d", finalBalance)
	}
}

func TestScenario3_CanceledRequestDoesNotSpendBalance(t *testing.T) {
	// 场景 3：
	// 用户 1 余额是 100。
	// 请求在进入扣款逻辑前已经被 cancel。
	// 正确结果：返回 408，并且不能扣余额，最终余额仍然是 100。
	// 这个场景用来理解 request context。
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

	t.Logf("场景 3：请求已经取消。HTTP 状态=%d，最终余额=%d", rec.Code, finalBalance)

	if finalBalance != 100 {
		t.Fatalf("expected canceled request to leave balance at 100, got %d", finalBalance)
	}
}
