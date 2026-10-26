// file: cmd/httpserver/main.go
package main

import (
	"log"
	"net/http"

	"httpServer/internal/http1"
)

const addr = ":8080"

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello\n"))
	})
	mux.HandleFunc("GET /get", handleGet)
	mux.HandleFunc("POST /post", handlePost)

	store := newUserStore()
	mux.HandleFunc("GET /users", store.list)
	mux.HandleFunc("POST /users", store.create)
	mux.HandleFunc("GET /users/{id}", store.get)
	mux.HandleFunc("PUT /users/{id}", store.update)
	mux.HandleFunc("DELETE /users/{id}", store.remove)

	log.Printf("listening on %s", addr)
	if err := http1.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
