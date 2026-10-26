package routeinventory

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// AssignmentReport is sent by DNS Control when an edge lease activates or
// deactivates. Identity and generation must match the current assignment.
type AssignmentReport struct {
	Domain     string `json:"domain"`
	AgentID    string `json:"agent_id"`
	PublicIP   string `json:"public_ip"`
	Generation int64  `json:"generation"`
	Active     bool   `json:"active"`
}

func (h Handler) ReportAssignment(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	if h.ControlKey == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")), []byte(h.ControlKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var report AssignmentReport
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&report); err != nil {
		http.Error(w, "invalid report", http.StatusBadRequest)
		return
	}
	report.Domain = strings.ToLower(strings.TrimSpace(report.Domain))
	report.AgentID = strings.TrimSpace(report.AgentID)
	ip := net.ParseIP(strings.TrimSpace(report.PublicIP))
	if report.Domain == "" || report.AgentID == "" || ip == nil || ip.To4() == nil || report.Generation <= 0 {
		http.Error(w, "invalid assignment identity", http.StatusBadRequest)
		return
	}
	state := "pending"
	if report.Active {
		state = "active"
	}
	offlineAfter := h.OfflineAfter
	if offlineAfter <= 0 {
		offlineAfter = 2 * time.Minute
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE network_route_assignments a SET
		state=$5, updated_at=now()
		FROM network_route_agent_ips i
		WHERE a.agent_ip_id=i.id AND a.domain=$1 AND i.agent_id=$2
		  AND i.public_ip=$3::inet AND a.generation=$4
		  AND ($5 <> 'active' OR (i.enabled=true AND i.last_seen_at > now() - $6::interval))
		  AND ($5='active' OR a.state<>$5 OR a.updated_at IS NULL)`,
		report.Domain, report.AgentID, ip.To4().String(), report.Generation, state, fmt.Sprintf("%f seconds", offlineAfter.Seconds()))
	if err != nil {
		http.Error(w, "assignment update failed", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		// A duplicate successful report is safe; stale generations must not be accepted.
		var current string
		err := h.DB.QueryRow(r.Context(), `SELECT a.state FROM network_route_assignments a
			JOIN network_route_agent_ips i ON i.id=a.agent_ip_id
			WHERE a.domain=$1 AND i.agent_id=$2 AND i.public_ip=$3::inet AND a.generation=$4
			  AND ($5 <> 'active' OR (i.enabled=true AND i.last_seen_at > now() - $6::interval))`,
			report.Domain, report.AgentID, ip.To4().String(), report.Generation, state, fmt.Sprintf("%f seconds", offlineAfter.Seconds())).Scan(&current)
		if err != nil || current != state {
			http.Error(w, "stale or unknown assignment", http.StatusConflict)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
