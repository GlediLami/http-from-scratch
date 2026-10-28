// file: cmd/httpserver/main.go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"httpServer/internal/http1"
)

var addr string

func main() {

	env := NewEnv()
	r := http1.NewRouter()
	port := env.Port
	addr = ":" + port
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	m := newMetrics(log)

	log.Info("listening", "port", port)

	r.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello\n"))
	})
	r.HandleFunc("GET /get", handleGet)
	r.HandleFunc("POST /post", handlePost)

	store := newUserStore()
	r.HandleFunc("GET /users", store.list)
	r.HandleFunc("POST /users", store.create)
	r.HandleFunc("GET /users/{id}", store.get)
	r.HandleFunc("PUT /users/{id}", store.update)
	r.HandleFunc("DELETE /users/{id}", store.remove)

	r.HandleFunc("GET /metrics", m.handler)
	r.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.middleware(r)

	log.Info("listening on %s", "addr", addr)
	if err := http1.ListenAndServe(addr, handler); err != nil {
		log.Error("Error while listening", "err", err)
	}
}
