package plans

import (
 "encoding/json"
 "log/slog"
 "net/http"
 "strings"
)

type Handler struct { repository *Repository; logger *slog.Logger }
func NewHandler(repository *Repository,logger *slog.Logger)*Handler{return &Handler{repository:repository,logger:logger}}

func (h *Handler) List(w http.ResponseWriter,r *http.Request){
 items,err:=h.repository.ListActive(r.Context())
 if err!=nil {h.logger.Error("list plans failed","error",err);http.Error(w,"internal server error",http.StatusInternalServerError);return}
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{"plans":items})
}

func (h *Handler) Get(w http.ResponseWriter,r *http.Request){
 code:=strings.TrimSpace(r.PathValue("code"))
 if code=="" {http.Error(w,"invalid plan code",http.StatusBadRequest);return}
 item,err:=h.repository.GetActiveByCode(r.Context(),code)
 if err!=nil {h.logger.Error("get plan failed","error",err);http.Error(w,"internal server error",http.StatusInternalServerError);return}
 if item==nil {http.Error(w,"plan not found",http.StatusNotFound);return}
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(item)
}
