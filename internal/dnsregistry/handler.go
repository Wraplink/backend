package dnsregistry

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type RegisterRequest struct {
	ID        string   `json:"id"`
	Region    string   `json:"region,omitempty"`
	Addresses []string `json:"addresses"`
}

type Handler struct {
	store        *Store
	controlKey   string
	offlineAfter time.Duration
}

func NewHandler(store *Store, controlKey string, offlineAfter time.Duration) *Handler {
	if store == nil {
		panic("dnsregistry: nil store")
	}
	return &Handler{
		store:        store,
		controlKey:   strings.TrimSpace(controlKey),
		offlineAfter: offlineAfter,
	}
}

func (h *Handler) Enabled() bool {
	return h.controlKey != ""
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	provided := r.Header.Get("X-Network-Control-Key")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.controlKey)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid network control key")
		return
	}

	var request RegisterRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	node, ok := h.store.Upsert(request.ID, request.Region, request.Addresses)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "id and at least one valid DNS IP address are required")
		return
	}

	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	provided := r.Header.Get("X-Network-Control-Key")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.controlKey)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid network control key")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": h.store.List(h.offlineAfter),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handler) AdminNodes(w http.ResponseWriter,r *http.Request) {
 nodes:=h.store.List(0)
 cutoff:=time.Now().UTC().Add(-h.offlineAfter)
 result:=make([]map[string]any,0,len(nodes))
 for _,node:=range nodes {
  online:=h.offlineAfter>0 && !node.LastSeenAt.Before(cutoff)
  result=append(result,map[string]any{
   "id":node.ID,"region":node.Region,"addresses":node.Addresses,
   "lastSeenAt":node.LastSeenAt,"online":online,
  })
 }
 writeJSON(w,http.StatusOK,map[string]any{"nodes":result})
}
