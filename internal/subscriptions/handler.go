package subscriptions

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"backend/internal/requestcontext"
 "backend/internal/starterpolicy"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		panic("subscriptions: nil service")
	}

	return &Handler{service: service}
}

type createSubscriptionBody struct {
	PlanCode string `json:"planCode"`
}

func (h *Handler) CreatePending(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body createSubscriptionBody

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}

	item, err := h.service.CreatePending(r.Context(), userID, body.PlanCode)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPlanCode):
			writeJSONError(w, http.StatusBadRequest, "planCode is required")
		case errors.Is(err, ErrPlanNotFound):
			writeJSONError(w, http.StatusNotFound, "plan not found")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not create subscription")
		}
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	items, err := h.service.List(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not list subscriptions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subscriptions": items,
	})
}

func (h *Handler) Current(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	item, err := h.service.Current(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not get current subscription")
		return
	}
	if item == nil {
		writeJSONError(w, http.StatusNotFound, "active subscription not found")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CancelPending(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	subscriptionID, err := uuid.Parse(chi.URLParam(r, "subscriptionID"))
	if err != nil || subscriptionID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid subscription ID")
		return
	}

	if err := h.service.CancelPending(r.Context(), userID, subscriptionID); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			writeJSONError(w, http.StatusNotFound, "subscription not found")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "could not cancel subscription")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "subscription cancelled successfully",
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


// ActivateStarter is authenticated by the /api/v1/me router and uses the
// request context's user identity rather than a caller-provided user ID.
func (h *Handler) ActivateStarter(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context())
 if !ok {writeJSONError(w,http.StatusUnauthorized,"authentication required");return}
 claimID,err:=uuid.Parse(chi.URLParam(r,"claimID"))
 if err!=nil || claimID==uuid.Nil {writeJSONError(w,http.StatusBadRequest,"invalid claim ID");return}
 result,err:=(starterpolicy.ActivationStore{DB:h.service.repository.DB()}).Activate(r.Context(),userID,claimID)
 if err!=nil {
  switch {
  case errors.Is(err,starterpolicy.ErrNotEligible):writeJSONError(w,http.StatusNotFound,"starter claim not found or unavailable")
  case errors.Is(err,starterpolicy.ErrReservationExpired):writeJSONError(w,http.StatusConflict,"starter reservation expired")
  case errors.Is(err,starterpolicy.ErrAlreadyActivated):writeJSONError(w,http.StatusConflict,"starter claim is not reserved")
  case errors.Is(err,starterpolicy.ErrActiveSubscription):writeJSONError(w,http.StatusConflict,"an active subscription already exists")
  default:writeJSONError(w,http.StatusInternalServerError,"could not activate starter plan")
  }
  return
 }
 writeJSON(w,http.StatusOK,result)
}


func (h *Handler) MyStarterClaims(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context())
 if !ok {writeJSONError(w,http.StatusUnauthorized,"authentication required");return}
 claims,err:=(starterpolicy.ActivationStore{DB:h.service.repository.DB()}).MyStarterClaims(r.Context(),userID)
 if err!=nil {writeJSONError(w,http.StatusInternalServerError,"could not list starter claims");return}
 writeJSON(w,http.StatusOK,map[string]any{"claims":claims})
}


// AdminStarterPolicies is protected by the router's admin authorization.
func (h *Handler) AdminStarterPolicies() starterpolicy.AdminHandler {
 return starterpolicy.AdminHandler{Store:starterpolicy.PolicyStore{DB:h.service.repository.DB()}}
}
