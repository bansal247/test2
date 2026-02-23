package test2

import (
	"errors"
	"sync"
)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Product struct {
	ID    string
	Name  string
	Stock int
}

type ReserveItem struct {
	ProductID string
	Quantity  int
}

type SafeInventoryService struct {
	mu       sync.RWMutex
	products map[string]*Product
}

func NewSafeInventoryService(products map[string]*Product) *SafeInventoryService {
	return &SafeInventoryService{products: products}
}

func (s *SafeInventoryService) GetStock(productID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	product := s.products[productID]
	if product == nil {
		return 0
	}
	return product.Stock
}

func (s *SafeInventoryService) Reserve(productID string, quantity int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	product := s.products[productID]
	if product == nil {
		return ErrProductNotFound
	}
	if product.Stock < quantity {
		return ErrInsufficientStock
	}

	product.Stock -= quantity
	return nil
}

func (s *SafeInventoryService) ReserveMultiple(items []ReserveItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate all items first (all-or-nothing check)
	for _, item := range items {
		product := s.products[item.ProductID]
		if product == nil {
			return ErrProductNotFound
		}
		if product.Stock < item.Quantity {
			return ErrInsufficientStock
		}
	}

	// Commit only after all checks pass
	for _, item := range items {
		s.products[item.ProductID].Stock -= item.Quantity
	}

	return nil
}