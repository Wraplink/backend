package dnsregistry

import (
 "encoding/json"
 "net/http"
 "time"
)

// AdminAgents exposes Backend-authoritative agent health to authenticated administrators.
// The caller must enforce administrator authorization at the router.
func (h *Handler) AdminAgents(w http.ResponseWriter, r *http.Request) {
 if h.db == nil { http.Error(w,"database unavailable",http.StatusServiceUnavailable);return }
 rows,err:=h.db.Query(r.Context(),`SELECT id,region,status,sni_capacity,route_capacity,registered_at,last_seen_at
 FROM network_agent_health ORDER BY id LIMIT 5001`)
 if err!=nil {http.Error(w,"agent health unavailable",http.StatusInternalServerError);return}
 defer rows.Close()
 type agent struct {
  ID string `json:"id"`
  Region string `json:"region"`
  Status string `json:"status"`
  SNICapacity int `json:"sniCapacity"`
  RouteCapacity int `json:"routeCapacity"`
  RegisteredAt time.Time `json:"registeredAt"`
  LastSeenAt time.Time `json:"lastSeenAt"`
  Online bool `json:"online"`
 }
 agents:=make([]agent,0)
 for rows.Next() {
  var a agent
  if err:=rows.Scan(&a.ID,&a.Region,&a.Status,&a.SNICapacity,&a.RouteCapacity,&a.RegisteredAt,&a.LastSeenAt);err!=nil {http.Error(w,"agent health unavailable",500);return}
  a.Online=a.Status=="online" && time.Since(a.LastSeenAt)<30*time.Second
  agents=append(agents,a)
  if len(agents)>5000 {http.Error(w,"agent inventory exceeds page limit",http.StatusServiceUnavailable);return}
 }
 if rows.Err()!=nil {http.Error(w,"agent health unavailable",500);return}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 _=json.NewEncoder(w).Encode(struct{Agents []agent `json:"agents"`}{agents})
}
