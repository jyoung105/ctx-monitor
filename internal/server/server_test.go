package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// buildHandler constructs the CORS-wrapped mux used by StartServer, allowing
// tests to exercise all routes without starting a real TCP listener.
func buildHandler(opts ServerOpts) http.Handler {
	mux := http.NewServeMux()

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

	return corsMiddleware(mux)
}

func newTestServer(t *testing.T) (*httptest.Server, ServerOpts) {
	t.Helper()
	opts := ServerOpts{
		HTMLTemplate: fallbackHTML,
		GetComposition: func() (interface{}, error) {
			return map[string]string{"tool": "claude"}, nil
		},
		GetSessions: func() (interface{}, error) {
			return []string{"session-1", "session-2"}, nil
		},
		GetSessionByID: func(id string) (interface{}, error) {
			return map[string]string{"id": id}, nil
		},
		GetTimeline: func(id string) (interface{}, error) {
			return map[string]string{"id": id, "type": "timeline"}, nil
		},
	}
	srv := httptest.NewServer(buildHandler(opts))
	t.Cleanup(srv.Close)
	return srv, opts
}

func TestGetRootReturns200WithHTML(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct[:9] != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestGetAPIDataReturns200WithJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/data")
	if err != nil {
		t.Fatalf("GET /api/data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Errorf("response body is not valid JSON: %v — body: %s", err, body)
	}
}

func TestGetAPISessionsReturns200WithJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestGetAPISessionByIDReturnsJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/session/test-id")
	if err != nil {
		t.Fatalf("GET /api/session/test-id: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Errorf("response is not valid JSON: %v — body: %s", err, body)
	}
	if result["id"] != "test-id" {
		t.Errorf("id = %v, want \"test-id\"", result["id"])
	}
}

func TestGetUnknownPathReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/unknown")
	if err != nil {
		t.Fatalf("GET /unknown: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestCORSHeadersPresent(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/", "/api/data", "/api/sessions"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()

		got := resp.Header.Get("Access-Control-Allow-Origin")
		if got != "*" {
			t.Errorf("GET %s: Access-Control-Allow-Origin = %q, want \"*\"", path, got)
		}
	}
}

func TestOPTIONSReturns204(t *testing.T) {
	srv, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/data", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /api/data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want \"*\"", got)
	}
}
