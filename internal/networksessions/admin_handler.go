package networksessions

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strconv"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

// AdminHandler is mounted ONLY inside the existing authenticated admin router.
// ActorID must resolve the authenticated administrator from trusted context.
type AdminHandler struct {
 Repository *Repository
 ActorID func(*http.Request) (uuid.UUID,error)
 HeartbeatTTL time.Duration
}
func writeJSON(w http.ResponseWriter,status int,v any) {
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(v)
}
func (h *AdminHandler) ListSessions(w http.ResponseWriter,r *http.Request) {
 if h.Repository==nil || h.ActorID==nil {http.Error(w,"unavailable",http.StatusServiceUnavailable);return}
 actor,err:=h.ActorID(r)
 if err!=nil || actor==uuid.Nil {http.Error(w,"forbidden",http.StatusForbidden);return}
 limit:=50
 if raw:=r.URL.Query().Get("limit");raw!="" {
  n,err:=strconv.Atoi(raw)
  if err!=nil || n<1 || n>500 {http.Error(w,"invalid limit",http.StatusBadRequest);return}
  limit=n
 }
 ttl:=h.HeartbeatTTL
 if ttl<=0 {ttl=45*time.Second}
 sessions,err:=h.Repository.ListOnline(r.Context(),time.Now().UTC(),ttl,limit)
 if err!=nil {http.Error(w,"could not list sessions",http.StatusInternalServerError);return}
 writeJSON(w,http.StatusOK,map[string]any{"items":sessions})
}
func (h *AdminHandler) Disconnect(w http.ResponseWriter,r *http.Request) {
 if h.Repository==nil || h.ActorID==nil {http.Error(w,"unavailable",http.StatusServiceUnavailable);return}
 actor,err:=h.ActorID(r)
 if err!=nil || actor==uuid.Nil {http.Error(w,"forbidden",http.StatusForbidden);return}
 sessionID,err:=uuid.Parse(chi.URLParam(r,"sessionID"))
 if err!=nil {http.Error(w,"invalid session ID",http.StatusBadRequest);return}
 var req struct {Reason string `json:"reason"`;IdempotencyKey string `json:"idempotencyKey"`}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&req);err!=nil {http.Error(w,"invalid JSON",http.StatusBadRequest);return}
 var trailing any
 if err:=decoder.Decode(&trailing);err!=io.EOF {http.Error(w,"trailing JSON not allowed",http.StatusBadRequest);return}
 command,err:=h.Repository.SubmitDisconnect(r.Context(),sessionID,actor,req.Reason,req.IdempotencyKey)
 if err!=nil {http.Error(w,"could not submit disconnect",http.StatusConflict);return}
 writeJSON(w,http.StatusAccepted,map[string]any{"commandId":command.ID,"status":command.State})
}
func (h *AdminHandler) CommandStatus(w http.ResponseWriter,r *http.Request) {
 if h.Repository==nil || h.ActorID==nil {http.Error(w,"unavailable",http.StatusServiceUnavailable);return}
 actor,err:=h.ActorID(r)
 if err!=nil || actor==uuid.Nil {http.Error(w,"forbidden",http.StatusForbidden);return}
 id,err:=uuid.Parse(chi.URLParam(r,"commandID"))
 if err!=nil {http.Error(w,"invalid command ID",http.StatusBadRequest);return}
 command,err:=h.Repository.GetCommand(r.Context(),id)
 if errors.Is(err,pgx.ErrNoRows) {http.NotFound(w,r);return}
 if err!=nil {http.Error(w,"could not load command",http.StatusInternalServerError);return}
 // Admin route permission checks are required at the router boundary.
 writeJSON(w,http.StatusOK,map[string]any{"commandId":command.ID,"status":command.State,"updatedAt":command.UpdatedAt,"errorCode":command.ErrorCode})
}
