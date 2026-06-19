package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

// mapCache is an in-memory Cache implementation used only in tests,
// so handler tests don't need a live Redis. It satisfies the Cache interface.
type mapCache struct {
	mu   sync.Mutex
	data map[int]Device
}

func newMapCache() *mapCache { return &mapCache{data: make(map[int]Device)} }

func (c *mapCache) Get(_ context.Context, id int) (*Device, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[id]
	if !ok {
		return nil, false
	}
	return &d, true
}

func (c *mapCache) Set(_ context.Context, d *Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[d.ID] = *d
}

func (c *mapCache) Invalidate(_ context.Context, id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, id)
}

// newTestRouter wires the real handlers with in-memory repo + cache —
// no Postgres, no Redis. This is the payoff of the interface design.
func newTestRouter() http.Handler {
	h := NewDeviceHandler(NewMemoryRepo(), newMapCache())
	r := chi.NewRouter()
	r.Get("/health", h.Health)
	r.Route("/devices", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
	return r
}

// --- helpers ---

func createDevice(t *testing.T, router http.Handler, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed create failed: status %d", rec.Code)
	}
}

func doGet(router http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestHealth(t *testing.T) {
	rec := doGet(newTestRouter(), "/health")
	if rec.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", rec.Code)
	}
}

func TestCreateDevice(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"valid", `{"name":"Pulse Oximeter","sku":"PX-100","category":"monitoring","quantity":40,"price":1299}`, http.StatusCreated},
		{"missing name", `{"sku":"PX-100"}`, http.StatusBadRequest},
		{"missing sku", `{"name":"Pulse Oximeter"}`, http.StatusBadRequest},
		{"malformed json", `{not json}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouter()
			req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestGetDevice(t *testing.T) {
	router := newTestRouter()
	createDevice(t, router, `{"name":"Pulse Oximeter","sku":"PX-100","category":"monitoring","quantity":40,"price":1299}`)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"existing", "/devices/1", http.StatusOK},
		{"missing", "/devices/999", http.StatusNotFound},
		{"non-integer id", "/devices/abc", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doGet(router, tt.path)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestGetDeviceCachePath(t *testing.T) {
	router := newTestRouter() // fresh router so cache starts empty
	createDevice(t, router, `{"name":"Pulse Oximeter","sku":"PX-100","category":"monitoring","quantity":40,"price":1299}`)

	first := doGet(router, "/devices/1")
	if first.Code != http.StatusOK {
		t.Fatalf("first get status = %d, want 200", first.Code)
	}
	if got := first.Header().Get("X-Cache"); got != "MISS" {
		t.Errorf("first read X-Cache = %q, want MISS", got)
	}

	second := doGet(router, "/devices/1")
	if got := second.Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("second read X-Cache = %q, want HIT", got)
	}
}

func TestListDevicesPagination(t *testing.T) {
	router := newTestRouter()
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"name":"Dev %d","sku":"SKU-%d","category":"monitoring","quantity":1,"price":1}`, i, i)
		createDevice(t, router, body)
	}

	tests := []struct {
		name     string
		path     string
		wantLen  int
	}{
		{"all", "/devices", 5},
		{"limit 2", "/devices?limit=2&offset=0", 2},
		{"offset past end", "/devices?limit=10&offset=10", 0},
		{"filter no match", "/devices?category=nonexistent", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doGet(router, tt.path)
			var got []Device
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}