package plans

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	repository *Repository
	logger     *slog.Logger
}

func NewHandler(
	repository *Repository,
	logger *slog.Logger,
) *Handler {
	if repository == nil {
		panic("plans: nil repository")
	}
	if logger == nil {
		panic("plans: nil logger")
	}

	return &Handler{
		repository: repository,
		logger:     logger,
	}
}

func (h *Handler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	items, err := h.repository.ListActive(r.Context())
	if err != nil {
		h.logger.Error("list plans failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not list plans")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plans": items,
	})
}

func (h *Handler) Get(
	w http.ResponseWriter,
	r *http.Request,
) {
	code := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "code")))
	if code == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid plan code")
		return
	}

	item, err := h.repository.GetActiveByCode(r.Context(), code)
	if err != nil {
		h.logger.Error("get plan failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not get plan")
		return
	}
	if item == nil {
		writeJSONError(w, http.StatusNotFound, "plan not found")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
