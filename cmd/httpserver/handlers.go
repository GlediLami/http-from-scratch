// file: cmd/httpserver/handlers.go
package main

import (
	"encoding/json"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"args": r.URL.Query(), "headers": r.Header, "host": r.Host,
		"origin": r.RemoteAddr, "url": r.RequestURI,
	})
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	resp := map[string]any{
		"args": r.URL.Query(), "headers": r.Header, "host": r.Host,
		"origin": r.RemoteAddr, "data": string(raw),
	}
	if r.Header.Get("Content-Type") == "application/json" && len(raw) > 0 {
		var js any
		if err := json.Unmarshal(raw, &js); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		resp["json"] = js
	}
	writeJSON(w, http.StatusOK, resp)
}
