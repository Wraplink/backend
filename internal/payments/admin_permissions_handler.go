package payments

import (
	"backend/internal/requestcontext"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
)

func (h *Handler) requireAccessAdmin(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	actor, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, 401, "authentication required")
		return uuid.Nil, false
	}
	var allowed bool
	if err := h.service.repository.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)", actor).Scan(&allowed); err != nil {
		writeJSONError(w, 500, "authorization unavailable")
		return uuid.Nil, false
	}
	if !allowed {
		writeJSONError(w, 403, "access administrator required")
		return uuid.Nil, false
	}
	return actor, true
}

const (
	PermissionUsers           = "users"
	PermissionOrders          = "orders"
	PermissionPaymentMethods  = "payment_methods"
	PermissionPlans           = "plans"
	PermissionStarterPlans    = "starter_plans"
	PermissionSupport         = "support"
	PermissionContactMessages = "contact_messages"
	PermissionNetworkMonitor  = "network_monitor"
	PermissionNetworkManage   = "network_manage"
	PermissionDomainPolicies  = "domain_policies"
)

var sectionPermissions = []string{
	PermissionUsers, PermissionOrders, PermissionPaymentMethods, PermissionPlans,
	PermissionStarterPlans, PermissionSupport, PermissionContactMessages,
	PermissionNetworkMonitor, PermissionNetworkManage, PermissionDomainPolicies,
}

// AdminCapabilities reports only the authenticated caller's effective permissions.
func (h *Handler) AdminCapabilities(w http.ResponseWriter, r *http.Request) {
	id, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, 401, "authentication required")
		return
	}
	var access, review, discount bool
	var sections []string
	if err := h.service.repository.db.QueryRow(r.Context(), `SELECT
  EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1),
  EXISTS(SELECT 1 FROM payment_review_admins WHERE user_id=$1),
  EXISTS(SELECT 1 FROM discount_admins WHERE user_id=$1),
  ARRAY(SELECT permission FROM admin_section_permissions WHERE user_id=$1 ORDER BY permission)`, id).
		Scan(&access, &review, &discount, &sections); err != nil {
		writeJSONError(w, 500, "authorization unavailable")
		return
	}
	if access {
		sections = append(append([]string{}, sectionPermissions...), "access")
	} else if slices.Contains(sections, PermissionNetworkManage) && !slices.Contains(sections, PermissionNetworkMonitor) {
		sections = append(sections, PermissionNetworkMonitor)
	}
	if review {
		sections = append(sections, "payment_review")
	}
	if discount {
		sections = append(sections, "discount_admin")
	}
	writeJSON(w, 200, map[string]any{"permissions": sections})
}

func (h *Handler) requireSection(w http.ResponseWriter, r *http.Request, permissions ...string) bool {
	id, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, 401, "authentication required")
		return false
	}
	var allowed bool
	err := h.service.repository.db.QueryRow(r.Context(), `SELECT
  EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)
  OR EXISTS(SELECT 1 FROM admin_section_permissions WHERE user_id=$1
    AND (permission=ANY($2) OR ('network_monitor'=ANY($2) AND permission='network_manage')))`, id, permissions).Scan(&allowed)
	if err != nil {
		writeJSONError(w, 500, "authorization unavailable")
		return false
	}
	if !allowed {
		writeJSONError(w, 403, "administrator section permission required")
	}
	return allowed
}

func (h *Handler) SectionMiddleware(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.requireSection(w, r, permissions...) {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func (h *Handler) ListAdminPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAccessAdmin(w, r); !ok {
		return
	}
	rows, err := h.service.repository.db.Query(r.Context(), `SELECT u.id,u.email,
 EXISTS(SELECT 1 FROM payment_review_admins p WHERE p.user_id=u.id),
 EXISTS(SELECT 1 FROM discount_admins d WHERE d.user_id=u.id),
 ARRAY(SELECT permission FROM admin_section_permissions a WHERE a.user_id=u.id ORDER BY permission)
 FROM users u WHERE EXISTS(SELECT 1 FROM payment_review_admins p WHERE p.user_id=u.id)
 OR EXISTS(SELECT 1 FROM discount_admins d WHERE d.user_id=u.id)
 OR EXISTS(SELECT 1 FROM admin_section_permissions a WHERE a.user_id=u.id)
 ORDER BY u.email LIMIT 500`)
	if err != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var email string
		var reviewer, discount bool
		var sections []string
		if err = rows.Scan(&id, &email, &reviewer, &discount, &sections); err != nil {
			writeJSONError(w, 500, "database error")
			return
		}
		result = append(result, map[string]any{"userId": id, "email": email, "paymentReview": reviewer, "discountAdmin": discount, "sections": sections})
	}
	if rows.Err() != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	writeJSON(w, 200, map[string]any{"users": result})
}

func (h *Handler) UpdateAdminPermission(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAccessAdmin(w, r)
	if !ok {
		return
	}
	target, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeJSONError(w, 400, "invalid user ID")
		return
	}
	permission := chi.URLParam(r, "permission")
	var table string
	switch permission {
	case "payment_review":
		table = "payment_review_admins"
	case "discount_admin":
		table = "discount_admins"
	default:
		if !slices.Contains(sectionPermissions, permission) {
			writeJSONError(w, 400, "invalid permission")
			return
		}
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil || body.Enabled == nil {
		writeJSONError(w, 400, "enabled boolean required")
		return
	}
	tx, err := h.service.repository.db.Begin(r.Context())
	if err != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize grants and revocations for the same account, including the audit writes.
	err = tx.QueryRow(r.Context(), "SELECT id FROM users WHERE id=$1 FOR UPDATE", target).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONError(w, 404, "user not found")
		return
	}
	if err != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	if table == "" {
		if *body.Enabled {
			_, err = tx.Exec(r.Context(), "INSERT INTO admin_section_permissions(user_id,permission) VALUES($1,$2) ON CONFLICT DO NOTHING", target, permission)
		} else {
			_, err = tx.Exec(r.Context(), "DELETE FROM admin_section_permissions WHERE user_id=$1 AND permission=$2", target, permission)
		}
	} else if *body.Enabled {
		_, err = tx.Exec(r.Context(), "INSERT INTO "+table+"(user_id) VALUES($1) ON CONFLICT DO NOTHING", target)
	} else {
		_, err = tx.Exec(r.Context(), "DELETE FROM "+table+" WHERE user_id=$1", target)
	}
	if err != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO admin_permission_events(actor_id,target_id,permission,enabled) VALUES($1,$2,$3,$4)", actor, target, permission, *body.Enabled)
	if err != nil {
		writeJSONError(w, 500, "audit write failed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSONError(w, 500, "database error")
		return
	}
	writeJSON(w, 200, map[string]any{"userId": target, "permission": permission, "enabled": *body.Enabled})
}
