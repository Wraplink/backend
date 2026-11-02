package plans

import (
	"encoding/json"
 "regexp"
 "time"
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

var validPlanCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
func (h *Handler) AdminList(w http.ResponseWriter,r *http.Request){
 items,err:=h.repository.ListAdmin(r.Context())
 if err!=nil{h.logger.Error("list admin plans failed","error",err);writeJSONError(w,500,"could not list plans");return}
 writeJSON(w,200,map[string]any{"plans":items})
}
func (h *Handler) AdminSave(w http.ResponseWriter,r *http.Request){
 r.Body=http.MaxBytesReader(w,r.Body,64<<10)
 var p AdminPlanInput
 if err:=json.NewDecoder(r.Body).Decode(&p);err!=nil{writeJSONError(w,400,"invalid JSON");return}
 p.Code=strings.ToLower(strings.TrimSpace(p.Code))
 p.Name=strings.TrimSpace(p.Name)
 p.Currency=strings.ToUpper(strings.TrimSpace(p.Currency))
 if !validPlanCode.MatchString(p.Code)||len(p.Name)==0||len(p.Name)>200||len(p.Description)>4000||p.PriceMinor<0||p.DurationDays<1||p.DurationDays>3650||p.DataLimitBytes<0||p.SortOrder<0||len(p.Currency)!=3{
 writeJSONError(w,400,"invalid plan fields");return
 }
 for _,c:=range p.Currency{if c<'A'||c>'Z'{writeJSONError(w,400,"invalid currency");return}}
 if p.PublishAt!=nil{t:=p.PublishAt.UTC();if t.Before(time.Date(2000,1,1,0,0,0,0,time.UTC)){writeJSONError(w,400,"invalid publication date");return};p.PublishAt=&t}
 if err:=h.repository.SaveAdmin(r.Context(),p);err!=nil{h.logger.Error("save admin plan failed","error",err);writeJSONError(w,500,"could not save plan");return}
 writeJSON(w,200,map[string]any{"saved":true})
}
