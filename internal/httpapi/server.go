// Package httpapi is the HTTP transport for the web UI. It wraps the same
// internal/service layer the Telegram bot uses, so every feature is available to
// both surfaces via one source of truth.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zp-bots-telegram/octopus-agile-bot/internal/service"
	"github.com/zp-bots-telegram/octopus-agile-bot/internal/session"
)

type Server struct {
	svc       *service.Service
	sessions  *session.Manager
	botToken  string
	log       *slog.Logger
	mux       *http.ServeMux
	devOrigin string // if set, permit CORS from this origin for SvelteKit dev
}

type Deps struct {
	Service   *service.Service
	Sessions  *session.Manager
	BotToken  string
	Log       *slog.Logger
	DevOrigin string
}

func New(d Deps) *Server {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		svc:       d.Service,
		sessions:  d.Sessions,
		botToken:  d.BotToken,
		log:       log,
		mux:       http.NewServeMux(),
		devOrigin: d.DevOrigin,
	}
	s.routes()
	return s
}

// Handler returns the server's root handler (with middlewares applied).
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.logMiddleware(s.mux))
}

// ServeHTTP lets Server satisfy http.Handler directly for simpler wiring.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler().ServeHTTP(w, r)
}

// ---- middlewares ---------------------------------------------------------

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "dur", time.Since(start))
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.devOrigin != "" && r.Header.Get("Origin") == s.devOrigin {
			w.Header().Set("Access-Control-Allow-Origin", s.devOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ctxKey marks the session claims stashed on the request context.
type ctxKey int

const claimsKey ctxKey = 1

// requireSession is a handler wrapper that 401s on missing/invalid cookies and
// otherwise passes the request through with claims attached.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.sessions.Verify(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func claimsOf(r *http.Request) session.Claims {
	if c, ok := r.Context().Value(claimsKey).(session.Claims); ok {
		return c
	}
	return session.Claims{}
}

// ---- helpers -------------------------------------------------------------

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
