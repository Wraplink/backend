package requestcontext

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct {
	name string
}

var (
	userIDKey    = contextKey{name: "user_id"}
	sessionIDKey = contextKey{name: "session_id"}
)

func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, false
	}

	return userID, true
}

func WithSessionID(ctx context.Context, sessionID uuid.UUID) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func SessionID(ctx context.Context) (uuid.UUID, bool) {
	sessionID, ok := ctx.Value(sessionIDKey).(uuid.UUID)
	if !ok || sessionID == uuid.Nil {
		return uuid.Nil, false
	}

	return sessionID, true
}
