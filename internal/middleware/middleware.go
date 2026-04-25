package middleware

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/services"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	HostKey      contextKey = "host"
)

// Chain applies multiple middleware to a handler
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Logger logs HTTP requests
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		log.Printf(
			"%s %s %s %d %s",
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
			wrapped.statusCode,
			time.Since(start),
		)
	})
}

// Recover recovers from panics
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// MethodOverride converts POST requests with _method form field to the specified HTTP method.
// This allows HTML forms to submit PUT/DELETE requests since forms only support GET/POST.
func MethodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Parse form to access _method field
			if err := r.ParseForm(); err == nil {
				method := r.FormValue("_method")
				if method == http.MethodPut || method == http.MethodDelete {
					r.Method = method
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isAPIRequest returns true if the request targets the JSON API (Bearer token auth).
func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

// ExtractSessionToken gets the session token from Bearer header or cookie.
// Returns the token and whether it came from a Bearer header.
func ExtractSessionToken(r *http.Request) (string, bool) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer "), true
	}
	if cookie, err := r.Cookie("session"); err == nil {
		return cookie.Value, false
	}
	return "", false
}

// RequireAuth ensures the user is authenticated.
// Supports both cookie-based sessions (browser) and Bearer token (API clients).
func RequireAuth(sessionService *services.SessionService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, isBearer := ExtractSessionToken(r)
			apiReq := isAPIRequest(r)

			if token == "" {
				if apiReq {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				} else {
					http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				}
				return
			}

			host, err := sessionService.ValidateSession(r.Context(), token)
			if err != nil {
				if apiReq {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				} else {
					// Clear invalid cookie (only relevant for cookie auth)
					if !isBearer {
						http.SetCookie(w, &http.Cookie{
							Name:     "session",
							Value:    "",
							Path:     "/",
							MaxAge:   -1,
							HttpOnly: true,
						})
					}
					http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				}
				return
			}

			ctx := context.WithValue(r.Context(), HostKey, host)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetHost retrieves the authenticated host from context
func GetHost(ctx context.Context) *services.HostWithTenant {
	host, ok := ctx.Value(HostKey).(*services.HostWithTenant)
	if !ok {
		return nil
	}
	return host
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	id, ok := ctx.Value(RequestIDKey).(string)
	if !ok {
		return ""
	}
	return id
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
