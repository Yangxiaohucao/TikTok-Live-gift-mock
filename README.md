# TikTok Live Gift Mock

Phase 1 learning goal: understand single-service concurrency correctness with a tiny Go HTTP API.

This project simulates a gift purchase endpoint:

```http
POST /gift
```

Request body:

```json
{
  "userId": 1,
  "streamerId": 100,
  "amount": 10
}
```

User `1` starts with balance `100`.

## Run The Server

```powershell
go run main.go
```

The server listens on:

```text
http://localhost:8080
```

## Run The Tests

```powershell
go test -v
```

Expected learning output:

```text
场景 1：20 个并发请求，每个扣 10。成功=10，余额不足失败=10，最终余额=0
场景 2：两个并发大额请求，每个扣 80。成功=1，余额不足失败=1，最终余额=20
场景 3：请求已经取消。HTTP 状态=408，最终余额=100
```

## What This Is Testing

### 1. Goroutine

In the tests, each `go func() { ... }()` simulates one request arriving at the same time as other requests.

For example, 20 goroutines means 20 users or requests trying to send gifts concurrently.

### 2. sync.WaitGroup

`sync.WaitGroup` lets the test wait until every goroutine has finished.

Without it, the test might check the final balance before all gift requests have completed.

### 3. Race Condition

A race condition can happen if two requests read and write the same balance at the same time.

Example:

```text
balance = 100

Request A sends 80
Request B sends 80

A reads balance = 100
B reads balance = 100

A thinks the user has enough money
B also thinks the user has enough money
```

If both requests are allowed to continue, the system may accept `160` coins of spending from a wallet that only had `100`.

That is unacceptable for a revenue system.

### 4. sync.Mutex

The critical section is:

```go
balance, ok := balances[req.UserID]

if balance < req.Amount {
    // reject the gift
}

balance -= req.Amount
balances[req.UserID] = balance
```

This whole block must behave like one atomic operation.

`sync.Mutex` makes sure only one request can check and update the wallet balance at a time.

### 5. context

`context` represents the lifetime of a request.

If the request is already canceled, `/gift` should not spend the user's balance.

The test proves this:

```text
场景 3：请求已经取消。HTTP 状态=408，最终余额=100
```

## Why This Matters

For a normal counter, a small mistake may only mean an inaccurate number.

For TikTok Live gifts, this touches:

- wallet balance
- gift payment
- streamer revenue
- transaction correctness

So the important rule is:

```text
checking balance + deducting balance must not run concurrently for the same wallet
```

Later, when this moves from an in-memory map to MySQL, the same idea becomes database transactions and row locks.
