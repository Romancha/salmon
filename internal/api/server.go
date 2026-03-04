package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/romancha/bear-sync/internal/api/docs"
	"github.com/romancha/bear-sync/internal/mapper"
	"github.com/romancha/bear-sync/internal/store"
)

type contextKey string

const consumerIDKey contextKey = "consumer_id"

// ConsumerIDFromContext extracts the consumer ID set by the auth middleware.
func ConsumerIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(consumerIDKey).(string)
	return v
}

const (
	syncStatusConflict      = "conflict"
	syncStatusPendingToBear = "pending_to_bear"
)

// Server holds the HTTP handler and dependencies.
type Server struct {
	router         chi.Router
	store          store.Store
	consumerTokens map[string]string // name → token
	bridgeToken    string
	attachmentsDir string
}

// NewServer creates a new API server with all routes configured.
func NewServer(s store.Store, consumerTokens map[string]string, bridgeToken, attachmentsDir string) *Server {
	srv := &Server{
		store:          s,
		consumerTokens: consumerTokens,
		bridgeToken:    bridgeToken,
		attachmentsDir: attachmentsDir,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)

	r.Get("/healthz", srv.healthCheck)

	r.Route("/api", func(r chi.Router) {
		r.Route("/notes", func(r chi.Router) {
			r.Use(srv.authMiddleware("consumer"))

			defaultLimit := bodyLimitMiddleware(1 << 20) // 1 MB

			r.With(defaultLimit).Get("/", srv.listNotes)
			r.With(defaultLimit).Get("/search", srv.searchNotes)
			r.With(defaultLimit).Get("/{id}", srv.getNote)
			r.With(idempotencyRequired, defaultLimit).Post("/", srv.createNote)
			r.With(idempotencyRequired, defaultLimit).Put("/{id}", srv.updateNote)
			r.With(idempotencyRequired, defaultLimit).Delete("/{id}", srv.trashNote)

			r.Route("/{noteID}/tags", func(r chi.Router) {
				r.With(idempotencyRequired, defaultLimit).Post("/", srv.addTag)
			})

			r.With(idempotencyRequired, defaultLimit).Post("/{id}/archive", srv.archiveNote)

			r.Route("/{noteID}/attachments", func(r chi.Router) {
				r.With(idempotencyRequired, bodyLimitMiddleware(10<<20)).Post("/", srv.addFile) // 10 MB
			})

			r.With(defaultLimit).Get("/{noteID}/backlinks", srv.listBacklinks)
		})

		r.Route("/tags", func(r chi.Router) {
			r.Use(srv.authMiddleware("consumer"))
			r.Use(bodyLimitMiddleware(1 << 20))

			r.Get("/", srv.listTags)
			r.With(idempotencyRequired).Put("/{id}", srv.renameTag)
			r.With(idempotencyRequired).Delete("/{id}", srv.deleteTag)
		})

		r.Route("/attachments", func(r chi.Router) {
			r.Use(srv.authMiddleware("consumer"))

			r.Get("/{id}", srv.getAttachment)
		})

		r.Route("/sync", func(r chi.Router) {
			r.With(srv.authMiddleware("any")).Get("/status", srv.syncStatus)

			r.Group(func(r chi.Router) {
				r.Use(srv.authMiddleware("bridge"))

				r.With(bodyLimitMiddleware(50<<20)).Post("/push", srv.syncPush)
				r.Get("/queue", srv.syncQueue)
				r.With(bodyLimitMiddleware(1<<20)).Post("/ack", srv.syncAck)
				r.With(bodyLimitMiddleware(100<<20)).Post("/attachments/{id}", srv.syncUploadAttachment)
				r.Get("/attachments/{id}", srv.syncDownloadAttachment)
			})
		})

		r.Route("/docs", func(r chi.Router) {
			r.Use(srv.authMiddleware("consumer"))
			r.Get("/*", httpSwagger.Handler(
				httpSwagger.URL("/api/docs/doc.json"),
			))
		})
	})

	srv.router = r

	return srv
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// healthCheck godoc
// @Summary Health check
// @Description Returns health status of the hub server
// @Tags System
// @Produce json
// @Success 200 {object} map[string]string
// @Router /healthz [get]
func (s *Server) healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
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
			case "consumer":
				consumerName := s.matchConsumerToken(token)
				if consumerName == "" {
					writeError(w, http.StatusForbidden, "invalid token for consumer scope")
					return
				}
				ctx := context.WithValue(r.Context(), consumerIDKey, consumerName)
				r = r.WithContext(ctx)
			case "bridge":
				if subtle.ConstantTimeCompare([]byte(token), []byte(s.bridgeToken)) != 1 {
					writeError(w, http.StatusForbidden, "invalid token for bridge scope")
					return
				}
			case "any":
				matched := s.matchConsumerToken(token) != ""
				validBR := subtle.ConstantTimeCompare([]byte(token), []byte(s.bridgeToken)) == 1
				if !matched && !validBR {
					writeError(w, http.StatusForbidden, "invalid token")
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

// matchConsumerToken returns the consumer name whose token matches, or "" if none match.
// Iterates all entries unconditionally to avoid timing side-channels.
func (s *Server) matchConsumerToken(token string) string {
	var matched string
	for name, t := range s.consumerTokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(t)) == 1 {
			matched = name
		}
	}
	return matched
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

func writeInternalError(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "error", err)
	writeError(w, http.StatusInternalServerError, msg)
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}

func generateID() string {
	id, _ := mapper.GenerateID() //nolint:errcheck // crypto/rand.Read never returns error on supported platforms

	return id
}
