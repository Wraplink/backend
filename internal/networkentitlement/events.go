package networkentitlement

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/requestcontext"
	"github.com/google/uuid"
)

const (
	defaultConnectionEventsPageSize = 10
	maxConnectionEvents             = 50
)

// ConnectionEvent records actual account actions, not simulated edge traffic.
type ConnectionEvent struct {
	ID    uuid.UUID `json:"id"`
	Type  string    `json:"type"`
	Title string    `json:"title"`
	Time  time.Time `json:"time"`
}

type ConnectionEventsPage struct {
	Events     []ConnectionEvent `json:"events"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// Events returns a small, user-scoped page in newest-first order.
// The opaque cursor avoids slow OFFSET scans and duplicate rows.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	limit := defaultConnectionEventsPageSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeJSONError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsed < maxConnectionEvents {
			limit = parsed
		} else {
			limit = maxConnectionEvents
		}
	}

	var before *eventCursor
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		cursor, err := parseEventCursor(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid events cursor")
			return
		}
		before = &cursor
	}

	page, err := h.service.repository.ListEventsPage(r.Context(), userID, limit, before)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not load connection events")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type eventCursor struct {
	Time time.Time
	ID   uuid.UUID
}

func encodeEventCursor(event ConnectionEvent) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(event.Time.UTC().Format(time.RFC3339Nano) + "|" + event.ID.String()),
	)
}

func parseEventCursor(value string) (eventCursor, error) {
	if len(value) > 200 {
		return eventCursor{}, fmt.Errorf("cursor too long")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return eventCursor{}, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return eventCursor{}, fmt.Errorf("invalid cursor format")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return eventCursor{}, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return eventCursor{}, err
	}
	return eventCursor{Time: timestamp, ID: id}, nil
}

func (r *Repository) ListEventsPage(ctx context.Context, userID uuid.UUID, limit int, before *eventCursor) (ConnectionEventsPage, error) {
	if limit < 1 || limit > maxConnectionEvents {
		limit = defaultConnectionEventsPageSize
	}
	// Fetch one extra row to determine whether another page exists.
	query := `
		SELECT id, type, title, created_at
		FROM connection_events
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`
	args := []any{userID, limit + 1}
	if before != nil {
		query = `
			SELECT id, type, title, created_at
			FROM connection_events
			WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`
		args = []any{userID, before.Time, before.ID, limit + 1}
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ConnectionEventsPage{}, err
	}
	defer rows.Close()

	page := ConnectionEventsPage{Events: make([]ConnectionEvent, 0, limit)}
	for rows.Next() {
		var event ConnectionEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Title, &event.Time); err != nil {
			return ConnectionEventsPage{}, err
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return ConnectionEventsPage{}, err
	}
	if len(page.Events) > limit {
		page.Events = page.Events[:limit]
		page.NextCursor = encodeEventCursor(page.Events[len(page.Events)-1])
	}
	return page, nil
}

func (r *Repository) RecordEvent(ctx context.Context, userID uuid.UUID, eventType, title string) error {
	_, err := r.db.Exec(ctx, `
        INSERT INTO connection_events (user_id, type, title)
        VALUES ($1, $2, $3)
    `, userID, eventType, title)
	return err
}

// An event logging outage must never report an already-completed connection
// change as a failed action.
func (h *Handler) recordEvent(ctx context.Context, userID uuid.UUID, eventType, title string) {
	if err := h.service.repository.RecordEvent(ctx, userID, eventType, title); err != nil {
		log.Printf("connection event write failed: %v", err)
	}
}
