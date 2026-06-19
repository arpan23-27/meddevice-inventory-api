package main

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepo struct {
	mu     sync.RWMutex
	data   map[int]Device
	nextID int
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{data: make(map[int]Device), nextID: 1}
}

func (r *MemoryRepo) Create(_ context.Context, d *Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d.ID = r.nextID
	r.nextID++
	d.UpdatedAt = time.Now()
	r.data[d.ID] = *d
	return nil
}

func (r *MemoryRepo) GetByID(_ context.Context, id int) (*Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &d, nil
}

func (r *MemoryRepo) List(_ context.Context, category string, limit, offset int) ([]Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Device
	for _, d := range r.data {
		if category == "" || d.Category == category {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *MemoryRepo) Update(_ context.Context, d *Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[d.ID]; !ok {
		return ErrNotFound
	}
	d.UpdatedAt = time.Now()
	r.data[d.ID] = *d
	return nil
}

func (r *MemoryRepo) Delete(_ context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[id]; !ok {
		return ErrNotFound
	}
	delete(r.data, id)
	return nil
}