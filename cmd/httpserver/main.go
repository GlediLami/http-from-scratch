// file: cmd/httpserver/main.go
package main

import (
	"log"
	"net/http"

	"httpServer/internal/http1"
)

const addr = ":8080"

func main() {
	m := newMetrics()
	r := http1.NewRouter()

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

	log.Printf("listening on %s", addr)
	if err := http1.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
