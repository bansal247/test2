# Answers

## Q1: Why does a locally declared mutex in `SafeReserve` make it completely useless?

A `sync.Mutex` achieves mutual exclusion by having all competing goroutines contend for the **same object in memory**. When one goroutine holds the lock, others block on their `Lock()` call against that same address.

When `var mu sync.Mutex` is declared inside the function body, Go allocates a fresh, zero-valued mutex on each invocation's stack frame. Every call gets its own independent mutex. Goroutine A locks `muA`; Goroutine B locks `muB`. These are different variables—neither goroutine ever sees the other's mutex, so neither ever blocks. The `Lock()`/`Unlock()` calls succeed immediately on every goroutine, providing exactly zero protection. The code races just as badly as the version with no locking at all.

The fix is straightforward: move the mutex to the struct as a field (`s.mu sync.Mutex`). All callers then contend for the same instance.

---

## Q2: Per-product locks in `ReserveMultiple` and deadlock

If each product has its own `sync.Mutex` and `ReserveMultiple` acquires them in the order the caller provides, the classic **deadlock from lock-ordering inversion** can occur:

1. Goroutine 1 locks Product A, then tries to lock Product B.
2. Goroutine 2 locks Product B, then tries to lock Product A.
3. Each goroutine is waiting for the lock the other holds. Neither can proceed—permanent deadlock.

**Prevention strategy:**

**Global single lock (used in this implementation):** Acquire `s.mu` (a single `RWMutex` on the service) for the entire `ReserveMultiple` operation. This is simpler and eliminates the ordering problem entirely, at the cost of slightly reduced concurrency when reservations touch disjoint sets of products.

---

## Q3: Why is the "release early" pattern worse than no locks?

```go
s.mu.Lock()
product := s.products[productID]
s.mu.Unlock()          // lock released here

if product.Stock < quantity {   // check happens outside the lock
    return ErrInsufficientStock
}

s.mu.Lock()
product.Stock -= quantity       // update happens under a second lock
s.mu.Unlock()
```

This is **worse** than no locking for two reasons:

1. **It reintroduces the TOCTOU race it appears to fix.** The check (`product.Stock < quantity`) executes *outside* any lock. Between the first `Unlock()` and the second `Lock()`, another goroutine can freely modify `product.Stock`. The "check then act" gap is still wide open.

2. **It introduces a false sense of security.** Code with no locks at least signals to the reader that synchronization is missing. This version *looks* correct—it has lock/unlock pairs everywhere—but the critical section is split across two separate acquisitions. A reviewer or the author may incorrectly trust this code is safe, delaying discovery of the bug.

The correct fix is a single `Lock()` held across both the check and the update:

```go
s.mu.Lock()
defer s.mu.Unlock()
if product.Stock < quantity {
    return ErrInsufficientStock
}
product.Stock -= quantity
```

---

## Q4: Does a clean `-race` run guarantee race-freedom?

**No.** The Go race detector is a dynamic analysis tool—it only reports races on code paths that are **actually executed** during that particular test run. It works by instrumenting memory accesses at compile time and tracking access history at runtime, flagging when two goroutines access the same memory without synchronization. It cannot reason about code paths that were never taken.

A clean `-race` result means "no races were detected in this execution," not "no races exist." Robust concurrency testing requires both the race detector *and* tests specifically designed to maximize concurrent interleaving—stress tests.