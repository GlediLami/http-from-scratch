package http1

import (
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestResponse_WireSerialization(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	res := NewResponse(serverConn)
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusCreated)

	payload := []byte(`{"created":true}`)

	go func() {
		defer serverConn.Close()
		_, _ = res.Write(payload)
		_ = res.Finish()
	}()

	buf := make([]byte, 512)
	n, _ := clientConn.Read(buf)
	raw := string(buf[:n])

	if !strings.HasPrefix(raw, "HTTP/1.1 201 Created\r\n") {
		t.Errorf("Expected 201 Created status line, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Content-Type: application/json\r\n") {
		t.Errorf("Expected Content-Type header, got:\n%s", raw)
	}
	if !strings.HasSuffix(raw, "\r\n\r\n"+string(payload)) {
		t.Errorf("Expected payload %q, got:\n%s", string(payload), raw)
	}
}

func TestResponse_DefaultStatus(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	res := NewResponse(serverConn)

	go func() {
		defer serverConn.Close()
		// Calling Write without WriteHeader should automatically default to 200 OK
		_, _ = res.Write([]byte("auto ok"))
		_ = res.Finish()
	}()

	buf := make([]byte, 512)
	n, _ := clientConn.Read(buf)
	raw := string(buf[:n])

	if !strings.HasPrefix(raw, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("Expected default status line 200 OK, got:\n%s", raw)
	}
}
