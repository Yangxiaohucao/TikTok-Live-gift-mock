//go:build broken

package main

import (
	"sync"
	"testing"
)

func TestBrokenScenario_TwoLargeGiftsWithoutMutex(t *testing.T) {
	// 这是一个故意写错的教学测试。
	//
	// 场景：
	// 用户余额是 100。
	// 两个请求同时送礼，每个请求扣 80。
	//
	// 正确业务结果应该是：
	// 1 个成功，1 个余额不足失败，最终余额是 20。
	//
	// 但是这个 broken 版本没有 Mutex。
	// 两个请求都可能先读到 balance = 100，然后都判断“余额足够”。
	// 这样系统就会错误地接受 160 的消费。

	unsafeBalance := 100

	requestAReadDone := make(chan int, 1)
	requestBReadDone := make(chan int, 1)
	bothRequestsHaveRead := make(chan struct{})

	var wg sync.WaitGroup
	results := make(chan bool, 2)

	spendWithoutMutex := func(readDone chan<- int) {
		defer wg.Done()

		// 错误点 1：读取余额时没有锁。
		balance := unsafeBalance
		readDone <- balance

		// 故意让两个请求都先完成读取，再继续扣款。
		// 这样可以稳定复现 race condition，而不是碰运气。
		<-bothRequestsHaveRead

		if balance < 80 {
			results <- false
			return
		}

		// 错误点 2：写回余额时也没有锁。
		unsafeBalance = balance - 80
		results <- true
	}

	wg.Add(2)
	go spendWithoutMutex(requestAReadDone)
	go spendWithoutMutex(requestBReadDone)

	requestAReadBalance := <-requestAReadDone
	requestBReadBalance := <-requestBReadDone
	close(bothRequestsHaveRead)

	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for ok := range results {
		if ok {
			successes++
		} else {
			failures++
		}
	}

	t.Logf("错误版本：Request A 读到余额=%d，Request B 读到余额=%d", requestAReadBalance, requestBReadBalance)
	t.Logf("错误版本：成功=%d，余额不足失败=%d，最终余额=%d", successes, failures, unsafeBalance)

	if successes != 1 || failures != 1 || unsafeBalance != 20 {
		t.Fatalf("这个失败是故意的：没有 Mutex 时，两个请求都可能成功，导致系统接受超额消费")
	}
}
