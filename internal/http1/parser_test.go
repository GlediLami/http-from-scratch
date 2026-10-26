package http1

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseRequest_Valid(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantMethod string
		wantURI    string
		wantProto  string
		wantHost   string
		wantBody   string
	}{
		{
			name: "Standard GET",
			raw: "GET /hello HTTP/1.1\r\n" +
				"Host: localhost:8080\r\n" +
				"User-Agent: test\r\n\r\n",
			wantMethod: "GET",
			wantURI:    "/hello",
			wantProto:  "HTTP/1.1",
			wantHost:   "localhost:8080",
			wantBody:   "",
		},
		{
			name: "POST with Content-Length",
			raw: "POST /users HTTP/1.1\r\n" +
				"Host: localhost:8080\r\n" +
				"Content-Type: application/json\r\n" +
				"Content-Length: 17\r\n\r\n" +
				`{"name":"Alice"}`,
			wantMethod: "POST",
			wantURI:    "/users",
			wantProto:  "HTTP/1.1",
			wantHost:   "localhost:8080",
			wantBody:   `{"name":"Alice"}`,
		},
		{
			name: "Chunked Transfer Encoding",
			raw: "POST /users HTTP/1.1\r\n" +
				"Host: localhost:8080\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"4\r\nWiki\r\n" +
				"6\r\npedia \r\n" +
				"0\r\n\r\n",
			wantMethod: "POST",
			wantURI:    "/users",
			wantProto:  "HTTP/1.1",
			wantHost:   "localhost:8080",
			wantBody:   "Wikipedia ",
		},
		{
			name: "Leading Empty Line Ignored (RFC 9112 §2.2)",
			raw: "\r\n" +
				"GET /hello HTTP/1.1\r\n" +
				"Host: localhost\r\n\r\n",
			wantMethod: "GET",
			wantURI:    "/hello",
			wantProto:  "HTTP/1.1",
			wantHost:   "localhost",
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.raw))
			req, err := ParseRequest(reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if req.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", req.Method, tt.wantMethod)
			}
			if req.RequestURI != tt.wantURI {
				t.Errorf("RequestURI = %q, want %q", req.RequestURI, tt.wantURI)
			}
			if req.Proto != tt.wantProto {
				t.Errorf("Proto = %q, want %q", req.Proto, tt.wantProto)
			}
			if req.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", req.Host, tt.wantHost)
			}

			body, _ := io.ReadAll(req.Body)
			if string(body) != tt.wantBody {
				t.Errorf("Body = %q, want %q", string(body), tt.wantBody)
			}
		})
	}
}

func TestParseRequest_RFCViolations(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantStatus int
	}{
		{
			name: "Whitespace before colon (RFC 9112 §5.1)",
			raw: "GET / HTTP/1.1\r\n" +
				"Host : localhost\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Obsolete line folding (RFC 9112 §5.2)",
			raw: "GET / HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				" X-Folded: invalid\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Multiple spaces in request line (RFC 9112 §3)",
			raw:        "GET   /index   HTTP/1.1\r\nHost: localhost\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing Host header in HTTP/1.1 (RFC 9112 §3.2.1)",
			raw:        "GET /index HTTP/1.1\r\nUser-Agent: test\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Conflicting TE and CL (RFC 9112 §6.1)",
			raw: "POST / HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"Content-Length: 5\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"0\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Bare CR character (RFC 9112 §2.2)",
			raw:        "GET / HTTP/1.1\r\nHost: local\rhost\r\n\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "HTTP/2.0 Major Version (RFC 9112 §2.3)",
			raw:        "GET / HTTP/2.0\r\nHost: localhost\r\n\r\n",
			wantStatus: http.StatusHTTPVersionNotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.raw))
			_, err := ParseRequest(reader)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			var httpErr *HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected error to be *HTTPError, got %T: %v", err, err)
			}

			if httpErr.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, tt.wantStatus)
			}
		})
	}
}
