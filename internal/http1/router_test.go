package http1

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testRouter() *Router {
	r := NewRouter()
	r.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	r.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Id", r.PathValue("id"))
		w.WriteHeader(200)
	})
	r.HandleFunc("DELETE /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	r.HandleFunc("GET /files/{path...}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.PathValue("path"))
		w.WriteHeader(200)
	})
	return r
}

func TestRouter(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
		wantID       string // X-Id header, "" if none
		wantAllow    string // Allow header, "" if none
	}{
		{"GET", "/users", 200, "", ""},
		{"GET", "/users/42", 200, "42", ""},
		{"HEAD", "/users/42", 200, "42", ""}, // HEAD served by GET
		{"DELETE", "/users/42", 204, "", ""},
		{"GET", "/files/a/b/c.txt", 200, "", ""},
		{"POST", "/users/42", 405, "", "DELETE, GET"},
		{"GET", "/nope", 404, "", ""},
	}
	r := testRouter()
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tt.wantStatus {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, rec.Code, tt.wantStatus)
		}
		if got := rec.Header().Get("X-Id"); got != tt.wantID {
			t.Errorf("%s %s: id = %q, want %q", tt.method, tt.path, got, tt.wantID)
		}
		if got := rec.Header().Get("Allow"); got != tt.wantAllow {
			t.Errorf("%s %s: Allow = %q, want %q", tt.method, tt.path, got, tt.wantAllow)
		}
	}
}
