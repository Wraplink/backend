package security

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			defer func() {
				if value := recover(); value != nil {
					logger.Error(
						"panic recovered",
						"request_id", GetRequestID(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"panic", value,
						"stack", string(debug.Stack()),
					)

					http.Error(
						w,
						`{"error":"internal server error"}`,
						http.StatusInternalServerError,
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
