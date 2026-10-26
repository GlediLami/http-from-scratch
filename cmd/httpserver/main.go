package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"httpServer/internal/http1"
)

const addr = ":8080"

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var (
	usersMu sync.RWMutex
	users   = []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	nextID = 3
)

func main() {
	// 1. Basic greeting endpoint
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		body := []byte("hello")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	// 2. In-memory /users endpoint (GET and POST)
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetUsers(w, r)
		case http.MethodPost:
			handleCreateUser(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Printf("Custom HTTP/1.1 server listening on %s...\n", addr)
	if err := http1.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	usersMu.RLock()
	data, err := json.Marshal(users)
	usersMu.RUnlock()

	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Name == "" {
		http.Error(w, `{"error":"Invalid request payload, 'name' is required"}`, http.StatusBadRequest)
		return
	}

	usersMu.Lock()
	newUser := User{
		ID:   nextID,
		Name: payload.Name,
	}
	nextID++
	users = append(users, newUser)
	usersMu.Unlock()

	data, err := json.Marshal(newUser)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}
