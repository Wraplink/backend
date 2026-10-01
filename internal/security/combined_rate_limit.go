package security

import "net/http"

func CombinedRateLimit(
	first func(*http.Request) string,
	firstLimiter *RateLimiter,
	second func(*http.Request) string,
	secondLimiter *RateLimiter,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			firstKey := first(r)

			if !firstLimiter.Allow(firstKey) {
				w.Header().Set(
					"Retry-After",
					"900",
				)

				writeRateLimitResponse(w)
				return
			}

			secondKey := second(r)

			if !secondLimiter.Allow(secondKey) {
				w.Header().Set(
					"Retry-After",
					"3600",
				)

				writeRateLimitResponse(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeRateLimitResponse(w http.ResponseWriter) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(http.StatusTooManyRequests)

	_, _ = w.Write(
		[]byte(`{"error":"too many requests"}`),
	)
}
