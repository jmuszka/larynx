package server

import (
	"context"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

const requestTimeout = 30 * time.Second

// recoverer recovers from panics in downstream handlers and responds with a
// JSON error instead of the plain-text body chi's built-in Recoverer writes.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			if rvr := recover(); rvr != nil {
				if rvr == http.ErrAbortHandler {
					panic(rvr)
				}
				s.logger.Error("panic recovered", "error", rvr, "stack", debug.Stack())
				if ww.Status() == 0 {
					s.writeJSONError(ww, http.StatusInternalServerError, "Internal server error")
				}
			}
		}()
		next.ServeHTTP(ww, r)
	})
}

// timeout cancels the request context after a deadline and responds with a
// JSON 504 when the handler timed out without having written anything.
func (s *Server) timeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r.WithContext(ctx))

		if ctx.Err() == context.DeadlineExceeded && ww.Status() == 0 {
			s.writeJSONError(ww, http.StatusGatewayTimeout, "Request timeout")
		}
	})
}
