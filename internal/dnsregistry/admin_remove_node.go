package dnsregistry

import (
 "errors"
 "log/slog"
 "net/http"
 "strings"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/jackc/pgx/v5"
)

// AdminRemoveNode deletes only stale registry entries. Route inventory and
// assignments must be explicitly retired before removing an edge agent.
func (h *Handler) AdminRemoveNode(w http.ResponseWriter, r *http.Request) {
 kind, id := chi.URLParam(r,"type"), strings.TrimSpace(chi.URLParam(r,"id"))
 if id=="" || len(id)>128 || strings.ContainsAny(id,"/\\") {writeJSONError(w,400,"invalid node id");return}
 if h.offlineAfter<=0 {writeJSONError(w,503,"offline threshold is not configured");return}
 switch kind {
 case "dns":
  found, removed := h.store.RemoveInactive(id,h.offlineAfter)
  if !found {writeJSONError(w,404,"node not found");return}
  if !removed {writeJSONError(w,409,"node is online");return}
 case "edge":
  if h.db==nil {writeJSONError(w,503,"database unavailable");return}
  tx,err:=h.db.Begin(r.Context())
  if err!=nil {writeJSONError(w,500,"database unavailable");return}
  defer tx.Rollback(r.Context())
  // Coordinate with inventory writes and serialize removal against heartbeat.
  if _,err=tx.Exec(r.Context(),"SELECT pg_advisory_xact_lock(722311)");err!=nil {writeJSONError(w,500,"node lock failed");return}
  var lastSeen time.Time
  err=tx.QueryRow(r.Context(),"SELECT last_seen_at FROM network_agent_health WHERE id=$1 FOR UPDATE",id).Scan(&lastSeen)
  if errors.Is(err,pgx.ErrNoRows) {writeJSONError(w,404,"node not found");return}
  if err!=nil {writeJSONError(w,500,"node lookup failed");return}
  if !lastSeen.Before(time.Now().UTC().Add(-h.offlineAfter)) {writeJSONError(w,409,"node is online");return}
  // Conservatively refuse deletion while ANY routing inventory exists.
  // Never cascade into leases, domain policies, or usage history.
  var inventoryExists bool
  err=tx.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM network_route_agent_ips WHERE agent_id=$1)",id).Scan(&inventoryExists)
  if err!=nil {writeJSONError(w,500,"inventory lookup failed");return}
  if inventoryExists {writeJSONError(w,409,"node has route inventory; retire assignments and inventory first");return}
  tag,err:=tx.Exec(r.Context(),"DELETE FROM network_agent_health WHERE id=$1 AND last_seen_at < $2",id,time.Now().UTC().Add(-h.offlineAfter))
  if err!=nil {writeJSONError(w,500,"node removal failed");return}
  if tag.RowsAffected()!=1 {writeJSONError(w,409,"node heartbeat changed; retry");return}
  if err=tx.Commit(r.Context());err!=nil {writeJSONError(w,500,"node removal commit failed");return}
 default:
  writeJSONError(w,400,"invalid node type");return
 }
 slog.Info("admin removed inactive network node","type",kind,"node_id",id)
 w.WriteHeader(http.StatusNoContent)
}
