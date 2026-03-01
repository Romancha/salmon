package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/romancha/bear-sync/internal/store"
)

// Server holds the HTTP handler and dependencies.
type Server struct {
	router         chi.Router
	store          store.Store
	openclawToken  string
	bridgeToken    string
	attachmentsDir string
}

// NewServer creates a new API server with all routes configured.
func NewServer(s store.Store, openclawToken, bridgeToken, attachmentsDir string) *Server {
	srv := &Server{
		store:          s,
		openclawToken:  openclawToken,
		bridgeToken:    bridgeToken,
		attachmentsDir: attachmentsDir,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)

	r.Route("/api", func(r chi.Router) {
		r.Route("/notes", func(r chi.Router) {
			r.Use(srv.authMiddleware("openclaw"))
			r.Use(bodyLimitMiddleware(1 << 20)) // 1 MB

			r.Get("/", srv.listNotes)
			r.Get("/search", srv.searchNotes)
			r.Get("/{id}", srv.getNote)
			r.With(idempotencyRequired).Post("/", srv.createNote)
			r.With(idempotencyRequired).Put("/{id}", srv.updateNote)
			r.With(idempotencyRequired).Delete("/{id}", srv.trashNote)

			r.Route("/{noteID}/tags", func(r chi.Router) {
				r.With(idempotencyRequired).Post("/", srv.addTag)
			})

			r.Get("/{noteID}/backlinks", srv.listBacklinks)
		})

		r.Route("/tags", func(r chi.Router) {
			r.Use(srv.authMiddleware("openclaw"))
			r.Use(bodyLimitMiddleware(1 << 20))

			r.Get("/", srv.listTags)
		})

		r.Route("/attachments", func(r chi.Router) {
			r.Use(srv.authMiddleware("openclaw"))

			r.Get("/{id}", srv.getAttachment)
		})

		r.Route("/sync", func(r chi.Router) {
			r.Use(srv.authMiddleware("bridge"))

			r.With(bodyLimitMiddleware(50 << 20)).Post("/push", srv.syncPush)
			r.Get("/queue", srv.syncQueue)
			r.With(bodyLimitMiddleware(1 << 20)).Post("/ack", srv.syncAck)
			r.With(bodyLimitMiddleware(100 << 20)).Post("/attachments/{id}", srv.syncUploadAttachment)
			r.Get("/status", srv.syncStatus)
		})
	})

	srv.router = r

	return srv
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// --- Middleware ---

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		//nolint:gosec // method and path are HTTP metadata, not user-controlled taint for log injection
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start).String(),
		)
	})
}

func (s *Server) authMiddleware(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if len(token) < 8 || token[:7] != "Bearer " {
				writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}

			token = token[7:]

			switch scope {
			case "openclaw":
				if token != s.openclawToken {
					writeError(w, http.StatusForbidden, "invalid token for openclaw scope")
					return
				}
			case "bridge":
				if token != s.bridgeToken {
					writeError(w, http.StatusForbidden, "invalid token for bridge scope")
					return
				}
			default:
				writeError(w, http.StatusInternalServerError, "unknown auth scope")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func idempotencyRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, http.StatusBadRequest, "Idempotency-Key header is required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read never returns error on supported platforms

	return hex.EncodeToString(b)
}
