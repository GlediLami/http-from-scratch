// file: cmd/httpserver/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"httpServer/internal/http1"
)

var addr string

func main() {
	// stop on Ctrl+C / SIGTERM so the span batch flushes before exit
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	env := NewEnv()
	r := http1.NewRouter()
	port := env.Port
	addr = ":" + port
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// tracing: exports spans over OTLP, configured via OTEL_* env vars.
	// A failure here is not fatal: the server still serves, just without traces.
	shutdownTracing, err := setupTracing(ctx)
	if err != nil {
		log.Error("tracing setup failed", "err", err)
	}

	m := newMetrics(log)

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

	go func() {
		log.Info("listening", "addr", addr)
		if err := http1.ListenAndServe(addr, handler); err != nil {
			log.Error("listen", "err", err)
			stop() // unblock main so we exit
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	if shutdownTracing != nil {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shCtx); err != nil {
			log.Error("tracing shutdown", "err", err)
		}
	}
}
