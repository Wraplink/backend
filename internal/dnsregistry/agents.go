package dnsregistry

import (
 "crypto/subtle"
 "encoding/json"
 "net/http"
 "strings"
 "time"
)

// AgentRegistration is the authenticated Backend heartbeat and registration contract.
type AgentRegistration struct {
 ID string `json:"agent_id"`
 Region string `json:"region"`
 Capacity struct { SNI int `json:"sni"`; Route int `json:"route"` } `json:"capacity"`
}
func (h *Handler) agentAuthorized(w http.ResponseWriter, r *http.Request) bool {
 if !h.Enabled() { writeJSONError(w, http.StatusNotFound,"not found"); return false }
 if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")), []byte(h.controlKey)) != 1 {
  writeJSONError(w,http.StatusUnauthorized,"invalid network control key"); return false
 }
 if h.db == nil { writeJSONError(w,http.StatusServiceUnavailable,"database unavailable"); return false }
 return true
}

func (h *Handler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
 if !h.agentAuthorized(w,r) { return }
 var req AgentRegistration
 decoder := json.NewDecoder(http.MaxBytesReader(w,r.Body,16*1024))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&req);err!=nil {writeJSONError(w,400,"invalid request");return}
 req.ID=strings.TrimSpace(req.ID)
 if req.ID=="" || len(req.ID)>128 || req.Capacity.SNI<0 || req.Capacity.Route<0 || req.Capacity.SNI+req.Capacity.Route==0 {
  writeJSONError(w,400,"invalid agent identity or capacity");return
 }
 _,err:=h.db.Exec(r.Context(),`INSERT INTO network_agent_health(id,region,sni_capacity,route_capacity,status,registered_at,last_seen_at,revision)
 VALUES ($1,$2,$3,$4,'online',now(),now(),nextval('network_agent_health_revision_seq'))
 ON CONFLICT (id) DO UPDATE SET region=EXCLUDED.region,sni_capacity=EXCLUDED.sni_capacity,route_capacity=EXCLUDED.route_capacity,
 status='online',last_seen_at=now(),revision=nextval('network_agent_health_revision_seq')`,req.ID,req.Region,req.Capacity.SNI,req.Capacity.Route)
 if err!=nil {writeJSONError(w,500,"agent registration failed");return}
 writeJSON(w,200,map[string]any{"ok":true})
}

func (h *Handler) HeartbeatAgent(w http.ResponseWriter,r *http.Request) {
 if !h.agentAuthorized(w,r) {return}
 var req struct {AgentID string `json:"agent_id"`}
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096)).Decode(&req);err!=nil {writeJSONError(w,400,"invalid request");return}
 tag,err:=h.db.Exec(r.Context(),`UPDATE network_agent_health SET status='online',last_seen_at=now(),
 revision=nextval('network_agent_health_revision_seq') WHERE id=$1`,strings.TrimSpace(req.AgentID))
 if err!=nil {writeJSONError(w,500,"heartbeat failed");return}
 if tag.RowsAffected()==0 {writeJSONError(w,404,"agent not registered");return}
 writeJSON(w,200,map[string]any{"ok":true})
}

// AgentSnapshot provides deterministic keyset pages. Revision changes force
// consumers to discard partial results and retry from the first page.
func (h *Handler) AgentSnapshot(w http.ResponseWriter,r *http.Request) {
 if !h.agentAuthorized(w,r) {return}
 cursor:=r.URL.Query().Get("cursor")
 if len(cursor)>128 {writeJSONError(w,400,"invalid cursor");return}
 var revision int64
 err:=h.db.QueryRow(r.Context(),`SELECT GREATEST(COALESCE(MAX(revision),0),1) FROM network_agent_health`).Scan(&revision)
 if err!=nil {writeJSONError(w,500,"snapshot unavailable");return}
 rows,err:=h.db.Query(r.Context(),`SELECT id,region,sni_capacity,route_capacity,status,registered_at,last_seen_at
 FROM network_agent_health WHERE id>$1 ORDER BY id LIMIT 501`,cursor)
 if err!=nil {writeJSONError(w,500,"snapshot unavailable");return}
 defer rows.Close()
 type agent struct {
  ID string `json:"id"`
  Region string `json:"region"`
  Capacity struct { SNI int `json:"sni"`; Route int `json:"route"` } `json:"capacity"`
  Status string `json:"status"`
  RegisteredAt time.Time `json:"registered_at"`
  LastSeenAt time.Time `json:"last_seen_at"`
 }
 agents:=make([]agent,0,500)
 next:=""
 for rows.Next() {
  var a agent
  if err:=rows.Scan(&a.ID,&a.Region,&a.Capacity.SNI,&a.Capacity.Route,&a.Status,&a.RegisteredAt,&a.LastSeenAt);err!=nil {
   writeJSONError(w,500,"snapshot unavailable");return
  }
  if len(agents)==500 {next=agents[499].ID;break}
  if h.offlineAfter>0 && time.Since(a.LastSeenAt)>h.offlineAfter {a.Status="offline"}
  agents=append(agents,a)
 }
 if rows.Err()!=nil {writeJSONError(w,500,"snapshot unavailable");return}
 rows.Close()
 // Recheck revision after reading to avoid returning a mixed page.
 var current int64
 if err:=h.db.QueryRow(r.Context(),`SELECT GREATEST(COALESCE(MAX(revision),0),1) FROM network_agent_health`).Scan(&current);err!=nil || current!=revision {
  writeJSONError(w,http.StatusConflict,"agent inventory changed; retry");return
 }
 response:=map[string]any{"revision":revision,"agents":agents}
 if next!="" {response["next_cursor"]=next}
 writeJSON(w,200,response)
}
