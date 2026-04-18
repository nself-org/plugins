package sdk

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// CORS is a legacy shim retained for backward source compatibility. It is a
// no-op so callers cannot accidentally wire up wildcard CORS on endpoints
// that carry credentials. New code must use the per-origin middleware at
// github.com/nself-org/nself-sdk/middleware.CORS with an explicit
// AllowedOrigins list.
//
// Deprecated: use middleware.CORS(middleware.CORSConfig{AllowedOrigins: ...}).
func CORS(next http.Handler) http.Handler {
	return next
}

// RequestID adds an X-Request-ID header to each request using chi's
// built-in request ID middleware.
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

// Logger logs each request's method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		log.Printf("%s %s %d %s",
			r.Method,
			r.URL.Path,
			ww.Status(),
			time.Since(start).Round(time.Millisecond),
		)
	})
}

// Recovery catches panics in downstream handlers and returns a 500 response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("plugin-sdk: panic recovered: %v\n%s", rec, debug.Stack())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
