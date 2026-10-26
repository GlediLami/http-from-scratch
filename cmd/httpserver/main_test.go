package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func resetState() {
	usersMu.Lock()
	defer usersMu.Unlock()
	users = []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	nextID = 3
}

func TestHandleHello(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("hello"))
	}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Errorf("expected 'hello', got %q", w.Body.String())
	}
}

func TestHandleGetUsers(t *testing.T) {
	resetState()

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	w := httptest.NewRecorder()

	handleGetUsers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result []User
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if len(result) != 2 || result[0].Name != "Alice" || result[1].Name != "Bob" {
		t.Errorf("unexpected users returned: %+v", result)
	}
}

func TestHandleCreateUser_Success(t *testing.T) {
	resetState()

	payload := `{"name":"Charlie"}`
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()

	handleCreateUser(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", w.Code)
	}

	var created User
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.ID != 3 || created.Name != "Charlie" {
		t.Errorf("unexpected user created: %+v", created)
	}

	// Verify state mutation
	usersMu.RLock()
	total := len(users)
	usersMu.RUnlock()
	if total != 3 {
		t.Errorf("expected in-memory users count 3, got %d", total)
	}
}

func TestHandleCreateUser_InvalidInput(t *testing.T) {
	resetState()

	// Missing 'name' field
	payload := `{"email":"test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()

	handleCreateUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", w.Code)
	}
}
