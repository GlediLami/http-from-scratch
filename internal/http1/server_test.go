package http1

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestHandleConnection_KeepAliveAndClose(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.Write([]byte("pong"))
	})

	go func() {
		_ = HandleConnection(serverConn, mux)
	}()

	clientReader := bufio.NewReader(clientConn)

	// 1. Send first request on connection (Keep-Alive by default in HTTP/1.1)
	req1 := "GET /ping HTTP/1.1\r\nHost: localhost\r\n\r\n"
	if _, err := clientConn.Write([]byte(req1)); err != nil {
		t.Fatalf("write req1 failed: %v", err)
	}

	res1, err := http.ReadResponse(clientReader, nil)
	if err != nil {
		t.Fatalf("read res1 failed: %v", err)
	}
	if res1.StatusCode != http.StatusOK {
		t.Errorf("req1 status = %d, want 200", res1.StatusCode)
	}

	// Drain res1.Body so the bytes don't bleed into the next response
	body1, err := io.ReadAll(res1.Body)
	if err != nil {
		t.Fatalf("read res1.Body failed: %v", err)
	}
	res1.Body.Close()
	if string(body1) != "pong" {
		t.Errorf("body1 = %q, want %q", string(body1), "pong")
	}

	// 2. Send second request with explicit Connection: close on the same connection
	req2 := "GET /ping HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := clientConn.Write([]byte(req2)); err != nil {
		t.Fatalf("write req2 failed: %v", err)
	}

	res2, err := http.ReadResponse(clientReader, nil)
	if err != nil {
		t.Fatalf("read res2 failed: %v", err)
	}
	if res2.StatusCode != http.StatusOK {
		t.Errorf("req2 status = %d, want 200", res2.StatusCode)
	}
	if !res2.Close && !strings.EqualFold(res2.Header.Get("Connection"), "close") {
		t.Errorf("expected response to indicate closed connection (res2.Close = true)")
	}

	// Drain res2.Body
	_, _ = io.ReadAll(res2.Body)
	res2.Body.Close()

	// 3. Confirm connection is closed by server
	oneByte := make([]byte, 1)
	_, err = clientConn.Read(oneByte)
	if err == nil {
		t.Errorf("expected EOF/closed connection after Connection: close")
	}
}

func TestHandleConnection_ParserErrorPropagation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	go func() {
		_ = HandleConnection(serverConn, http.DefaultServeMux)
	}()

	// Send invalid request (space before colon)
	badReq := "GET / HTTP/1.1\r\nHost : localhost\r\n\r\n"
	_, _ = clientConn.Write([]byte(badReq))

	clientReader := bufio.NewReader(clientConn)
	resp, err := http.ReadResponse(clientReader, nil)
	if err != nil {
		t.Fatalf("failed reading error response: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}
