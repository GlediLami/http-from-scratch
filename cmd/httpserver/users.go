// file: cmd/httpserver/users.go
package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type userStore struct {
	mu     sync.Mutex
	users  map[int]User
	nextID int
}

func newUserStore() *userStore {
	return &userStore{users: make(map[int]User), nextID: 1}
}

// file: cmd/httpserver/users.go

func (s *userStore) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

func (s *userStore) create(w http.ResponseWriter, r *http.Request) {
	var in User
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if in.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	in.ID = s.nextID
	s.nextID++
	s.users[in.ID] = in
	s.mu.Unlock()

	w.Header().Set("Location", "/users/"+strconv.Itoa(in.ID))
	writeJSON(w, http.StatusCreated, in)
}

// file: cmd/httpserver/users.go

func (s *userStore) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	u, found := s.users[id]
	s.mu.Unlock()
	if !found {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *userStore) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in User
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	_, found := s.users[id]
	if found {
		in.ID = id
		s.users[id] = in
	}
	s.mu.Unlock()
	if !found {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *userStore) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	_, found := s.users[id]
	delete(s.users, id)
	s.mu.Unlock()
	if !found {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id must be a number", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
