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
 rows,err:=h.db.Query(r.Context(),`SELECT h.id,h.region,h.status,h.sni_capacity,h.route_capacity,h.registered_at,h.last_seen_at,
 EXISTS (SELECT 1 FROM network_route_agent_ips ip WHERE ip.agent_id=h.id AND ip.enabled=true)
 FROM network_agent_health h ORDER BY h.id LIMIT 5001`)
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
  RouteInventoryAvailable bool `json:"routeInventoryAvailable"`
  FullRouteStatus string `json:"fullRouteStatus"`
 }
 agents:=make([]agent,0)
 for rows.Next() {
  var a agent
  if err:=rows.Scan(&a.ID,&a.Region,&a.Status,&a.SNICapacity,&a.RouteCapacity,&a.RegisteredAt,&a.LastSeenAt,&a.RouteInventoryAvailable);err!=nil {http.Error(w,"agent health unavailable",500);return}
  a.Online=h.offlineAfter>0 && a.Status=="online" && time.Since(a.LastSeenAt)<h.offlineAfter
  switch {
  case !a.Online: a.FullRouteStatus="offline"
  case a.RouteCapacity<=0: a.FullRouteStatus="disabled"
  case !a.RouteInventoryAvailable: a.FullRouteStatus="unavailable"
  default: a.FullRouteStatus="provisioned"
  }
  agents=append(agents,a)
  if len(agents)>5000 {http.Error(w,"agent inventory exceeds page limit",http.StatusServiceUnavailable);return}
 }
 if rows.Err()!=nil {http.Error(w,"agent health unavailable",500);return}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 _=json.NewEncoder(w).Encode(struct{Agents []agent `json:"agents"`}{agents})
}
