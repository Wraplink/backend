package networkentitlement

import (
    "context"
    "log"
    "net/http"
    "time"

    "backend/internal/requestcontext"
    "github.com/google/uuid"
)

const maxConnectionEvents = 50

// ConnectionEvent records actual account actions, not simulated edge traffic.
type ConnectionEvent struct {
    ID uuid.UUID `json:"id"`
    Type string `json:"type"`
    Title string `json:"title"`
    Time time.Time `json:"time"`
}

// Events lists only the authenticated user's events, newest first.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
    userID, ok := requestcontext.UserID(r.Context())
    if !ok {
        writeJSONError(w, http.StatusUnauthorized, "authentication required")
        return
    }
    events, err := h.service.repository.ListEvents(r.Context(), userID, maxConnectionEvents)
    if err != nil {
        writeJSONError(w, http.StatusInternalServerError, "could not load connection events")
        return
    }
    writeJSON(w, http.StatusOK, events)
}

func (r *Repository) ListEvents(ctx context.Context, userID uuid.UUID, limit int) ([]ConnectionEvent, error) {
    if limit < 1 || limit > maxConnectionEvents {
        limit = maxConnectionEvents
    }
    rows, err := r.db.Query(ctx, `
        SELECT id, type, title, created_at
        FROM connection_events
        WHERE user_id = $1
        ORDER BY created_at DESC, id DESC
        LIMIT $2
    `, userID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    events := make([]ConnectionEvent, 0)
    for rows.Next() {
        var event ConnectionEvent
        if err := rows.Scan(&event.ID, &event.Type, &event.Title, &event.Time); err != nil {
            return nil, err
        }
        events = append(events, event)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    return events, nil
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
