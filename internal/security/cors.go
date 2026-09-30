package security

import (
	"net/http"
	"strings"
)

type CORS struct {
	allowedOrigins map[string]struct{}
}

func NewCORS(origins []string) *CORS {
	allowed := make(map[string]struct{}, len(origins))

	for _, origin := range origins {
		origin = strings.TrimSpace(origin)

		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return &CORS{
		allowedOrigins: allowed,
	}
}

func (c *CORS) Middleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		origin := r.Header.Get("Origin")

		if origin != "" {
			if _, ok := c.allowedOrigins[origin]; ok {
				w.Header().Set(
					"Access-Control-Allow-Origin",
					origin,
				)

				w.Header().Set(
					"Vary",
					"Origin",
				)

				w.Header().Set(
					"Access-Control-Allow-Credentials",
					"true",
				)

				w.Header().Set(
					"Access-Control-Allow-Headers",
					"Content-Type, Authorization, X-Request-ID",
				)

				w.Header().Set(
					"Access-Control-Allow-Methods",
					"GET, POST, PUT, PATCH, DELETE, OPTIONS",
				)
			}
		}

		if r.Method == http.MethodOptions {
			if origin == "" {
				http.Error(
					w,
					"forbidden",
					http.StatusForbidden,
				)
				return
			}

			if _, ok := c.allowedOrigins[origin]; !ok {
				http.Error(
					w,
					"forbidden",
					http.StatusForbidden,
				)
				return
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
