package sdk

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// pluginAllowedOrigins returns the per-request origin allowlist populated
// from PLUGIN_ALLOWED_ORIGINS (csv). An empty env var means no cross-origin
// browser requests are allowed. Wildcard ("*") is rejected: CORS with
// Allow-Origin: * cannot be combined with credentialed requests, and a
// shared-library default of "*" is inherited by every downstream plugin.
func pluginAllowedOrigins() map[string]bool {
	set := make(map[string]bool)
	for _, o := range strings.Split(os.Getenv("PLUGIN_ALLOWED_ORIGINS"), ",") {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "" || o == "*" {
			continue
		}
		set[o] = true
	}
	return set
}

// CORS enforces explicit per-origin CORS for every plugin built on this
// SDK. If the request Origin is not in the PLUGIN_ALLOWED_ORIGINS
// allowlist, CORS response headers are NOT emitted and the browser blocks
// the response. This default is safe for any plugin; consumers that need
// a different policy should register their own middleware instead of
// chaining this one.
//
// Server-to-server endpoints (webhooks, internal APIs) do not set an
// Origin header; this middleware is a no-op for them.
func CORS(next http.Handler) http.Handler {
	allowed := pluginAllowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && allowed[strings.ToLower(origin)]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			if !originAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
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

// AllowedCallers enforces the inter-plugin X-Source-Plugin allowlist. When
// StrictPluginAuth is true (the default in production), every inbound request
// must carry an X-Source-Plugin header whose value is listed in
// cfg.AllowedCallers. Missing or unauthorised callers receive 403. The 403
// response body is intentionally terse to avoid leaking allowlist contents.
//
// In dev environments set STRICT_PLUGIN_AUTH=false to disable the check and
// allow unrestricted local testing. Never disable in production.
//
// Wire this middleware after Recovery and before application handlers:
//
//	r.Use(sdk.Recovery, sdk.AllowedCallers(cfg), sdk.CORS, sdk.Logger)
//
// S43-T02.
func AllowedCallers(cfg *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.StrictPluginAuth {
				// Dev bypass — no check applied.
				next.ServeHTTP(w, r)
				return
			}

			caller := strings.TrimSpace(strings.ToLower(r.Header.Get("X-Source-Plugin")))
			if caller == "" {
				log.Printf("plugin-sdk: AllowedCallers: missing X-Source-Plugin header from %s %s", r.Method, r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if !cfg.AllowedCallers[caller] {
				log.Printf("plugin-sdk: AllowedCallers: caller %q not in allowlist for %s %s", caller, r.Method, r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
