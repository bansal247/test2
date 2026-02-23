package test2

import (
	"sync"
	"sync/atomic"
	"testing"
)

func newTestService(stocks map[string]int) *SafeInventoryService {
	products := make(map[string]*Product, len(stocks))
	for id, stock := range stocks {
		products[id] = &Product{ID: id, Name: id, Stock: stock}
	}
	return NewSafeInventoryService(products)
}


func TestReserve_ConcurrentOversell(t *testing.T) {
	const stock = 100
	const goroutines = 200

	svc := newTestService(map[string]int{"widget": stock})

	var (
		wg       sync.WaitGroup
		successes int64
		failures  int64
	)

	// Use a barrier so all goroutines start at roughly the same time.
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := svc.Reserve("widget", 1); err == nil {
				atomic.AddInt64(&successes, 1)
			} else {
				atomic.AddInt64(&failures, 1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if successes != stock {
		t.Errorf("expected exactly %d successes, got %d (failures: %d)", stock, successes, failures)
	}
	if failures != goroutines-stock {
		t.Errorf("expected exactly %d failures, got %d", goroutines-stock, failures)
	}
	if remaining := svc.GetStock("widget"); remaining != 0 {
		t.Errorf("expected 0 remaining stock, got %d", remaining)
	}
}

func TestReserveMultiple_Atomicity(t *testing.T) {
	svc := newTestService(map[string]int{
		"A": 10,
		"B": 5,
	})

	// This reservation requires 8 of A and 8 of B.
	// B only has 5, so the whole thing must fail.
	items := []ReserveItem{
		{ProductID: "A", Quantity: 8},
		{ProductID: "B", Quantity: 8},
	}

	err := svc.ReserveMultiple(items)
	if err == nil {
		t.Fatal("expected ReserveMultiple to fail, but it succeeded")
	}

	// Both products must be completely unchanged.
	if stockA := svc.GetStock("A"); stockA != 10 {
		t.Errorf("product A stock should still be 10, got %d", stockA)
	}
	if stockB := svc.GetStock("B"); stockB != 5 {
		t.Errorf("product B stock should still be 5, got %d", stockB)
	}
}
