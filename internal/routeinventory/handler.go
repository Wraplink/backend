package routeinventory

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB         *pgxpool.Pool
	ControlKey string
 OfflineAfter time.Duration
 DNSControlURL string
 DNSControlToken string
}

type InventoryRequest struct {
	AgentID   string   `json:"agentId"`
	PublicIPs []string `json:"publicIps"`
}

func (h Handler) RegisterInventory(w http.ResponseWriter, r *http.Request) {
	if h.ControlKey == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")), []byte(h.ControlKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req InventoryRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.AgentID) == "" || len(req.AgentID) > 128 || len(req.PublicIPs) > 256 {
		http.Error(w, "invalid inventory", http.StatusBadRequest)
		return
	}
	seen := make(map[string]bool)
	ips := make([]string, 0, len(req.PublicIPs))
	for _, address := range req.PublicIPs {
		ip := net.ParseIP(strings.TrimSpace(address))
		if ip == nil || ip.To4() == nil {
			http.Error(w, "invalid IPv4", http.StatusBadRequest)
			return
		}
		address = ip.To4().String()
		if !seen[address] {
			seen[address] = true
			ips = append(ips, address)
		}
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, "database unavailable", 500)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize competing inventory updates across all agents.
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(722311)"); err != nil {
		http.Error(w, "lock failed", 500)
		return
	}
	for _, address := range ips {
		var owner string
		err = tx.QueryRow(r.Context(), "SELECT agent_id FROM network_route_agent_ips WHERE public_ip=$1::inet", address).Scan(&owner)
		if err == nil && owner != req.AgentID {
			http.Error(w, "public IP already registered to another agent", 409)
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "inventory lookup failed", 500)
			return
		}
	}
	// Withdraw stale addresses rather than silently reassigning them.
	if _, err = tx.Exec(r.Context(), "UPDATE network_route_agent_ips SET enabled=false WHERE agent_id=$1", req.AgentID); err != nil {
		http.Error(w, "inventory update failed", 500)
		return
	}
	for _, address := range ips {
		if _, err = tx.Exec(r.Context(), `INSERT INTO network_route_agent_ips(agent_id,public_ip,enabled,last_seen_at)
 VALUES($1,$2::inet,true,now()) ON CONFLICT(public_ip) DO UPDATE SET enabled=true,last_seen_at=now()`, req.AgentID, address); err != nil {
			http.Error(w, "inventory write failed", 500)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "inventory commit failed", 500)
		return
	}
	if h.OfflineAfter > 0 {
  if err:=Reconcile(r.Context(),h.DB,h.OfflineAfter,h.DNSControlURL,h.DNSControlToken);err!=nil {
   http.Error(w,"inventory saved but route reconciliation failed",http.StatusServiceUnavailable)
   return
  }
 }
 w.Header().Set("Content-Type", "application/json")
 json.NewEncoder(w).Encode(map[string]any{"agentId": req.AgentID, "publicIps": ips, "lastSeenAt": time.Now().UTC()})
}

func (h Handler) AdminInventory(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT i.agent_id,host(i.public_ip),i.enabled,i.last_seen_at,
 COALESCE(a.domain,''),COALESCE(a.state,'')
 FROM network_route_agent_ips i LEFT JOIN network_route_assignments a ON a.agent_ip_id=i.id
 ORDER BY i.agent_id,i.public_ip`)
	if err != nil {
		http.Error(w, "inventory query failed", 500)
		return
	}
	defer rows.Close()
	type item struct {
		AgentID    string     `json:"agentId"`
		PublicIP   string     `json:"publicIp"`
		Enabled    bool       `json:"enabled"`
		LastSeenAt *time.Time `json:"lastSeenAt"`
		Domain     string     `json:"domain"`
		State      string     `json:"state"`
	}
	result := make([]item, 0)
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.AgentID, &v.PublicIP, &v.Enabled, &v.LastSeenAt, &v.Domain, &v.State); err != nil {
			http.Error(w, "inventory scan failed", 500)
			return
		}
		result = append(result, v)
	}
	if rows.Err() != nil {
		http.Error(w, "inventory scan failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"addresses": result})
}
