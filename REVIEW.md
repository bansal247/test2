# Race Condition Review

## Race Condition 1: Concurrent Map Reads and Writes in `GetStock`

- **Code:** `product := s.products[productID]`
- **What happens:** Go's built-in map is not safe for concurrent use. A goroutine reading the map at the same time another goroutine writes it (e.g., via `Reserve`) triggers undefined behavior—Go's runtime will `panic: concurrent map read and map write` in many cases, or silently corrupt internal map state.
- **Production scenario:** Goroutine A calls `GetStock("SKU-1")` and is mid-read of the map's internal hash table. Simultaneously, Goroutine B calls `Reserve("SKU-2")`, which writes to the same map. The map's internal rehashing during the write corrupts the read, causing a panic or an incorrect result.
- **Fix approach:** Acquire `s.mu.RLock()` before reading and release with `defer s.mu.RUnlock()`.

---

## Race Condition 2: Time-Of-Check-Time-Of-Use in `Reserve`

- **Code:**
  ```go
  if product.Stock < quantity {   // CHECK
      return ErrInsufficientStock
  }
  product.Stock -= quantity       // USE
  ```
- **What happens:** The check and the update are two separate operations with no lock held between them. Another goroutine can modify `product.Stock` in the gap, making the check stale by the time the update executes.
- **Production scenario:** Product has 1 unit of stock. Goroutine A checks `1 < 1` → false, passes. Before it executes `Stock -= 1`, Goroutine B also checks `1 < 1` → false, passes. Both goroutines then decrement, leaving `Stock = -1`. Two customers have "purchased" the last item.
- **Fix approach:** Hold a write lock (`s.mu.Lock()`) for the entire check-and-update block, making it a single atomic critical section.

---

## Race Condition 3: Non-Atomic Check-Then-Modify in `ReserveMultiple`

- **Code:**
  ```go
  // Check phase (no lock held across iterations)
  for _, item := range items {
      if product.Stock < item.Quantity { ... }
  }
  // Modify phase
  for _, item := range items {
      s.products[item.ProductID].Stock -= item.Quantity
  }
  ```
- **What happens:** Two separate loops with no atomic guarantee between them means another goroutine can deplete stock after the check loop passes but before the modify loop runs. Additionally, the two loops themselves are separate, so the check can pass for product A, then another goroutine steals A's stock, then A is over-decremented.
- **Production scenario:** Cart 1 and Cart 2 both want to reserve the last 5 units of Product A. Both carts pass the check loop (`5 >= 5`). Cart 1's modify loop runs first: `Stock = 0`. Cart 2's modify loop runs next: `Stock = -5`. Both orders are confirmed despite only one having sufficient inventory.
- **Fix approach:** Hold a single write lock for the combined check-and-modify operation. Validate all items first under the lock; only if all checks pass, commit the modifications.

---

## Race Condition 4: Local Mutex in `SafeReserve` (Mutex Per Call, Not Per Service)

- **Code:**
  ```go
  func (s *InventoryService) SafeReserve(...) error {
      var mu sync.Mutex   // <-- new mutex created on every call
      mu.Lock()
      defer mu.Unlock()
      ...
  }
  ```
- **What happens:** A `sync.Mutex` only provides mutual exclusion when **all** competing goroutines lock the **same** mutex instance. Here, a brand-new, independent mutex is created inside the function on every invocation. Each goroutine locks its own private mutex, which no other goroutine can see—so the `Lock()`/`Unlock()` calls are no-ops from a concurrency standpoint.
- **Production scenario:** Goroutine A calls `SafeReserve("SKU-1", 1)`, creating `muA`. Goroutine B calls `SafeReserve("SKU-1", 1)` at the same time, creating `muB`. Both goroutines immediately acquire their own uncontested mutex and proceed in parallel. No mutual exclusion exists; both goroutines race on `product.Stock` identically to the unlocked `Reserve` function.
- **Fix approach:** The mutex must be a **shared** field on the struct (`s.mu sync.Mutex`), so every call to `SafeReserve` on the same service instance competes for the same lock.