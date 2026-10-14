package edgeaccess

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"backend/internal/requestcontext"
	"backend/internal/security"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service      *Service
	trustedProxy *security.TrustedProxy
}

func NewHandler(service *Service, trustedProxy *security.TrustedProxy) *Handler {
	if service == nil {
		panic("edgeaccess: nil service")
	}
	if trustedProxy == nil {
		panic("edgeaccess: nil trusted proxy")
	}
	return &Handler{service: service, trustedProxy: trustedProxy}
}

func (h *Handler) InternalEnabled() bool {
	return h.service.InternalEnabled()
}

func (h *Handler) Provision(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	clientIP := h.trustedProxy.ClientIP(r)
	if clientIP == nil {
		writeJSONError(w, http.StatusBadRequest, "client IP could not be determined")
		return
	}

	item, err := h.service.Provision(r.Context(), userID, clientIP.String(), r.URL.Query().Get("region"))
	if err != nil {
		switch {
		case errors.Is(err, ErrNoUsableSubscription):
			writeJSONError(w, http.StatusConflict, "an active subscription with remaining quota is required")
		case errors.Is(err, ErrNoEdgeCapacity):
			writeJSONError(w, http.StatusServiceUnavailable, "no edge capacity is currently available")
		case errors.Is(err, ErrInvalidClientIP):
			writeJSONError(w, http.StatusBadRequest, "invalid client IP")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not provision network access")
		}
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (h *Handler) Current(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.service.Current(r.Context(), userID)
	if errors.Is(err, ErrAssignmentNotFound) {
		writeJSONError(w, http.StatusNotFound, "network access is not provisioned")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not get provisioned network access")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Release(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.service.Release(r.Context(), userID)
	if errors.Is(err, ErrAssignmentNotFound) {
		writeJSONError(w, http.StatusNotFound, "network access is not provisioned")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not release network access")
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (h *Handler) RegisterEdge(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeInternal(w, r) {
		return
	}
	var body RegisterEdgeRequest
	if !decodeJSON(w, r, &body, 64*1024) {
		return
	}
	item, err := h.service.RegisterEdge(r.Context(), body)
	if errors.Is(err, ErrInvalidEdgeRequest) {
		writeJSONError(w, http.StatusBadRequest, "invalid edge registration")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not register edge")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeInternal(w, r) {
		return
	}
	item, err := h.service.Heartbeat(r.Context(), chi.URLParam(r, "edgeCode"))
	if errors.Is(err, ErrEdgeNotFound) {
		writeJSONError(w, http.StatusNotFound, "edge not found")
		return
	}
	if errors.Is(err, ErrInvalidEdgeRequest) {
		writeJSONError(w, http.StatusBadRequest, "invalid edge code")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not record edge heartbeat")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeInternal(w, r) {
		return
	}
	after, err := parseInt64Query(r, "after", 0)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid after revision")
		return
	}
	limit64, err := parseInt64Query(r, "limit", 200)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	item, err := h.service.Sync(r.Context(), chi.URLParam(r, "edgeCode"), after, int(limit64))
	if errors.Is(err, ErrEdgeNotFound) {
		writeJSONError(w, http.StatusNotFound, "edge not found")
		return
	}
	if errors.Is(err, ErrInvalidEdgeRequest) {
		writeJSONError(w, http.StatusBadRequest, "invalid sync request")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not build edge sync")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeInternal(w, r) {
		return
	}
	var body AckRequest
	if !decodeJSON(w, r, &body, 16*1024) {
		return
	}
	err := h.service.Acknowledge(r.Context(), chi.URLParam(r, "edgeCode"), body)
	if errors.Is(err, ErrRevisionNotFound) {
		writeJSONError(w, http.StatusNotFound, "sync revision not found")
		return
	}
	if errors.Is(err, ErrInvalidEdgeRequest) {
		writeJSONError(w, http.StatusBadRequest, "invalid acknowledgement")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not acknowledge sync event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": body.Success, "revision": body.Revision})
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeInternal(w, r) {
		return
	}
	var body ActivityRequest
	if !decodeJSON(w, r, &body, 256*1024) {
		return
	}
	err := h.service.RecordActivity(r.Context(), chi.URLParam(r, "edgeCode"), body)
	if errors.Is(err, ErrEdgeNotFound) {
		writeJSONError(w, http.StatusNotFound, "edge not found")
		return
	}
	if errors.Is(err, ErrInvalidEdgeRequest) {
		writeJSONError(w, http.StatusBadRequest, "invalid activity request")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not record edge activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(body.Items)})
}

func (h *Handler) authorizeInternal(w http.ResponseWriter, r *http.Request) bool {
	if !h.service.InternalEnabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return false
	}
	if !h.service.Authorize(r.Header.Get("X-Edge-Key")) {
		writeJSONError(w, http.StatusUnauthorized, "invalid edge key")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func parseInt64Query(r *http.Request, key string, fallback int64) (int64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
