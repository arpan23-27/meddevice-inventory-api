package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type DeviceHandler struct {
	repo  DeviceRepository
	cache Cache
}

func NewDeviceHandler(repo DeviceRepository, cache Cache) *DeviceHandler {
	return &DeviceHandler{repo: repo, cache: cache}
}

func (h *DeviceHandler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DeviceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var d Device
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if d.Name == "" || d.SKU == "" {
		writeErr(w, http.StatusBadRequest, "name and sku are required")
		return
	}
	if err := h.repo.Create(r.Context(), &d); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create device")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *DeviceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	if d, ok := h.cache.Get(r.Context(), id); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, http.StatusOK, d)
		return
	}
	d, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "database error")
		return
	}
	h.cache.Set(r.Context(), d)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, http.StatusOK, d)
}

func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	limit := atoiDefault(r.URL.Query().Get("limit"), 20)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)
	devices, err := h.repo.List(r.Context(), category, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "database error")
		return
	}
	if devices == nil {
		devices = []Device{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (h *DeviceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	var d Device
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	d.ID = id
	err = h.repo.Update(r.Context(), &d)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "database error")
		return
	}
	h.cache.Invalidate(r.Context(), id)
	writeJSON(w, http.StatusOK, d)
}

func (h *DeviceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	err = h.repo.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "database error")
		return
	}
	h.cache.Invalidate(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}