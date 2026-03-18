package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const fallbackHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>ctx-monitor</title>
<style>body{font-family:system-ui,sans-serif;max-width:640px;margin:60px auto;color:#333}h1{color:#D4603A}code{background:#f5f5f5;padding:2px 6px;border-radius:3px}.endpoint{margin:8px 0}</style>
</head><body>
<h1>ctx-monitor</h1>
<p>Dashboard template not found. The API is available:</p>
<div class="endpoint"><code>GET /api/data</code> — Current context composition</div>
<div class="endpoint"><code>GET /api/sessions</code> — List sessions</div>
<div class="endpoint"><code>GET /api/session/:id</code> — Session details</div>
<div class="endpoint"><code>GET /api/timeline/:id</code> — Token growth timeline</div>
</body></html>`

// ServerOpts holds configuration for the HTTP server.
type ServerOpts struct {
	Port           int
	GetComposition func() (interface{}, error)
	GetSessions    func() (interface{}, error)
	GetSessionByID func(id string) (interface{}, error)
	GetTimeline    func(id string) (interface{}, error)
	HTMLTemplate   string // embedded HTML content for dashboard
}

// writeJSON marshals v with indentation and writes it as application/json.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data) //nolint:errcheck
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// corsMiddleware adds CORS headers to every response and handles OPTIONS preflight.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-cache")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// StartServer starts the HTTP server and blocks until ctx is cancelled.
func StartServer(opts ServerOpts, ctx context.Context) error {
	mux := http.NewServeMux()

	// GET / — serve dashboard HTML
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		html := opts.HTMLTemplate
		if html == "" {
			html = fallbackHTML
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	})

	// GET /api/data — current context composition
	mux.HandleFunc("GET /api/data", func(w http.ResponseWriter, r *http.Request) {
		if opts.GetComposition == nil {
			writeError(w, http.StatusInternalServerError, "GetComposition not configured")
			return
		}
		result, err := opts.GetComposition()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// GET /api/sessions — list sessions
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if opts.GetSessions == nil {
			writeError(w, http.StatusInternalServerError, "GetSessions not configured")
			return
		}
		result, err := opts.GetSessions()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// GET /api/session/{id} — session details
	mux.HandleFunc("GET /api/session/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if opts.GetSessionByID == nil {
			writeError(w, http.StatusInternalServerError, "GetSessionByID not configured")
			return
		}
		result, err := opts.GetSessionByID(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// GET /api/timeline/{id} — token growth timeline
	mux.HandleFunc("GET /api/timeline/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if opts.GetTimeline == nil {
			writeError(w, http.StatusInternalServerError, "GetTimeline not configured")
			return
		}
		result, err := opts.GetTimeline(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	handler := corsMiddleware(mux)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", opts.Port),
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "ctx-monitor dashboard: http://localhost:%d\n", opts.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
