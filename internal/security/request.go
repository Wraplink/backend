package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

type requestIDKey struct{}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		requestID := r.Header.Get("X-Request-ID")

		if !isValidRequestID(requestID) {
			requestID = generateRequestID()
		}

		ctx := context.WithValue(
			r.Context(),
			requestIDKey{},
			requestID,
		)

		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

func GetRequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func generateRequestID() string {
	var value [16]byte

	if _, err := rand.Read(value[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}

	return hex.EncodeToString(value[:])
}

func isValidRequestID(value string) bool {
	if len(value) < 16 || len(value) > 128 {
		return false
	}

	for _, r := range value {
		if !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_') {
			return false
		}
	}

	return true
}
