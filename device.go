package main 

import "time"

type  Device  struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	SKU       string    `json:"sku"`
	Quantity  int       `json:"quantity"`
	Price     float64   `json:"price"`
	UpdatedAt time.Time `json:"updated_at"`
}