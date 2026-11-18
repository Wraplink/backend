package networksessions

import (
 "encoding/json"
 "io"
 "net/http"
 "time"

 "github.com/google/uuid"
 "github.com/go-chi/chi/v5"
)

// AgentHandler authenticates each request using a separately provisioned agent token.
type AgentHandler struct {
 Repository *Repository
}
func (h *AgentHandler) agentID(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool) {
 if h.Repository==nil {http.Error(w,"unavailable",http.StatusServiceUnavailable);return uuid.Nil,false}
 id,err:=h.Repository.AuthenticateAgent(r)
 if err!=nil {http.Error(w,"unauthorized",http.StatusUnauthorized);return uuid.Nil,false}
 return id,true
}
func (h *AgentHandler) Report(w http.ResponseWriter,r *http.Request) {
 agentID,ok:=h.agentID(w,r);if !ok{return}
 var body struct {
  Sessions []struct {
   SessionID string `json:"sessionId"`
   UserID uuid.UUID `json:"userId"`
   SubscriptionID uuid.UUID `json:"subscriptionId"`
   ClientIP string `json:"clientIp"`
   Mode string `json:"mode"`
   Protocol string `json:"protocol"`
   ConnectedAt time.Time `json:"connectedAt"`
   LastActivityAt time.Time `json:"lastActivityAt"`
   UploadBytes int64 `json:"uploadBytes"`
   DownloadBytes int64 `json:"downloadBytes"`
  } `json:"sessions"`
 }
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10));decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&body);err!=nil || len(body.Sessions)==0 || len(body.Sessions)>100 {
  http.Error(w,"invalid session batch",http.StatusBadRequest);return
 }
 var trailing any
 if err:=decoder.Decode(&trailing);err!=io.EOF {http.Error(w,"trailing JSON not allowed",http.StatusBadRequest);return}
 now:=time.Now().UTC()
 ids:=make([]uuid.UUID,0,len(body.Sessions))
 for _,v:=range body.Sessions {
  if v.ConnectedAt.After(now.Add(5*time.Second)) || v.LastActivityAt.After(now.Add(5*time.Second)) {
   http.Error(w,"invalid timestamps",http.StatusBadRequest);return
  }
  id,err:=h.Repository.UpsertSession(r.Context(),PersistentSession{
   EdgeNodeID:agentID,AgentSessionID:v.SessionID,UserID:v.UserID,SubscriptionID:v.SubscriptionID,
   ClientIP:v.ClientIP,Mode:v.Mode,Protocol:v.Protocol,ConnectedAt:v.ConnectedAt,
   LastActivityAt:v.LastActivityAt,LastReportAt:now,UploadBytes:v.UploadBytes,DownloadBytes:v.DownloadBytes,
  })
  if err!=nil {http.Error(w,"session rejected",http.StatusConflict);return}
  ids=append(ids,id)
 }
 writeJSON(w,http.StatusOK,map[string]any{"sessionIds":ids})
}
func (h *AgentHandler) Commands(w http.ResponseWriter,r *http.Request) {
 agentID,ok:=h.agentID(w,r);if !ok{return}
 commands,err:=h.Repository.PendingForAgent(r.Context(),agentID,100)
 if err!=nil {http.Error(w,"commands unavailable",http.StatusInternalServerError);return}
 writeJSON(w,http.StatusOK,map[string]any{"commands":commands})
}
func (h *AgentHandler) Acknowledge(w http.ResponseWriter,r *http.Request) {
 agentID,ok:=h.agentID(w,r);if !ok{return}
 commandID,err:=uuid.Parse(chi.URLParam(r,"commandID"))
 if err!=nil {http.Error(w,"invalid command ID",http.StatusBadRequest);return}
 var body struct {Status CommandStatus `json:"status"`;ErrorCode string `json:"errorCode"`}
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));dec.DisallowUnknownFields()
 if err:=dec.Decode(&body);err!=nil {http.Error(w,"invalid acknowledgement",http.StatusBadRequest);return}
 var trailing any
 if err:=dec.Decode(&trailing);err!=io.EOF {http.Error(w,"trailing JSON not allowed",http.StatusBadRequest);return}
 if body.Status!=CommandSucceeded && body.Status!=CommandFailed {http.Error(w,"invalid status",http.StatusBadRequest);return}
 if err:=h.Repository.Acknowledge(r.Context(),agentID,commandID,body.Status,body.ErrorCode);err!=nil {
  http.Error(w,"acknowledgement rejected",http.StatusConflict);return
 }
 writeJSON(w,http.StatusOK,map[string]any{"commandId":commandID,"status":body.Status})
}
