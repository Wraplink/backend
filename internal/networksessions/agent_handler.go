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
func (h *AgentHandler) agentID(w http.ResponseWriter,r *http.Request)(string,bool) {
 if h.Repository==nil {http.Error(w,"unavailable",http.StatusServiceUnavailable);return "",false}
 id,err:=h.Repository.AuthenticateAgent(r)
 if err!=nil {http.Error(w,"unauthorized",http.StatusUnauthorized);return "",false}
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
 // Validate the complete batch before writing any session.
 seen:=make(map[string]struct{},len(body.Sessions))
 for _,v:=range body.Sessions {
  if _,exists:=seen[v.SessionID];exists {
   http.Error(w,"duplicate session ID in batch",http.StatusBadRequest);return
  }
  seen[v.SessionID]=struct{}{}
  if v.UploadBytes<0 || v.DownloadBytes<0 || (v.Mode!="sni" && (v.UserID==uuid.Nil || v.SubscriptionID==uuid.Nil)) {
   http.Error(w,"negative usage counters",http.StatusBadRequest);return
  }
  if err:=validate(Session{
   AgentID:agentID,SessionID:v.SessionID,UserID:v.UserID.String(),
   SubscriptionID:v.SubscriptionID.String(),ClientIP:v.ClientIP,Mode:v.Mode,
   Protocol:v.Protocol,ConnectedAt:v.ConnectedAt,LastActivityAt:v.LastActivityAt,
  },now);err!=nil {
   http.Error(w,"invalid session",http.StatusBadRequest);return
  }
 }
 ids:=make([]uuid.UUID,0,len(body.Sessions))
 for _,v:=range body.Sessions {
  // SNI identity is determined by backend entitlements, never by agent input.
  if v.Mode=="sni" {
   userID,subscriptionID,err:=h.Repository.ResolveSNIOwner(r.Context(),v.ClientIP)
   if err!=nil {http.Error(w,"SNI ownership is ambiguous or unavailable",http.StatusConflict);return}
   v.UserID,v.SubscriptionID=userID,subscriptionID
  }
  id,err:=h.Repository.UpsertSession(r.Context(),PersistentSession{
   AgentID:agentID,AgentSessionID:v.SessionID,UserID:v.UserID,SubscriptionID:v.SubscriptionID,
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
 // Expose only the fields required to terminate the agent-local session.
 // Administrator identity, reason and idempotency key stay server-side.
 type disconnectInstruction struct {
  CommandID uuid.UUID `json:"commandId"`
  SessionID string `json:"agentSessionId"`
 }
 instructions:=make([]disconnectInstruction,0,len(commands))
 for _,command:=range commands {
  instructions=append(instructions,disconnectInstruction{
   CommandID:command.ID,SessionID:command.AgentSessionID,
  })
 }
 writeJSON(w,http.StatusOK,map[string]any{"commands":instructions})
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
