package http1

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func ListenAndServe(addr string, handler http.Handler) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	if handler == nil {
		handler = http.DefaultServeMux
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}

		go HandleConnection(conn, handler)
	}
}

func HandleConnection(conn net.Conn, mux http.Handler) error {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		req, err := ParseRequest(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			var httpErr *HTTPError
			if errors.As(err, &httpErr) {
				writeError(conn, httpErr.StatusCode, httpErr.Message)
			} else {
				writeError(conn, http.StatusBadRequest, err.Error())
			}
			return err
		}

		// Initialize Response from response.go
		res := NewResponse(conn)
		res.Proto = req.Proto
		res.CloseConn = req.Close ||
			strings.EqualFold(req.Header.Get("Connection"), "close") ||
			(req.ProtoMajor == 1 && req.ProtoMinor == 0)

		// Dispatch request to handler
		mux.ServeHTTP(res, req)

		// RFC 9112 §6.3: If neither Content-Length nor Transfer-Encoding is set,
		// close connection so the client knows the body is complete.
		if res.Header().Get("Content-Length") == "" && res.Header().Get("Transfer-Encoding") == "" {
			res.CloseConn = true
		}

		if err := res.Finish(); err != nil {
			return err
		}

		if res.CloseConn {
			return nil
		}
	}
}

// writeError sends an RFC-compliant plaintext response directly over the socket.
func writeError(conn net.Conn, statusCode int, message string) {
	body := message + "\n"
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		statusCode,
		http.StatusText(statusCode),
		len(body),
		body,
	)
	_, _ = conn.Write([]byte(resp))
}

// ============================================================================
// ResponseWriter Implementation (implements http.ResponseWriter)
// ============================================================================

type responseWriter struct {
	conn        net.Conn
	bufWriter   *bufio.Writer
	header      http.Header
	wroteHeader bool
	statusCode  int
	proto       string
	closeConn   bool
}

func newResponseWriter(conn net.Conn, proto string, closeConn bool) *responseWriter {
	return &responseWriter{
		conn:      conn,
		bufWriter: bufio.NewWriter(conn),
		header:    make(http.Header),
		proto:     proto,
		closeConn: closeConn,
	}
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.flushHeader()
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.bufWriter.Write(p)
}

func (w *responseWriter) flushHeader() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	if w.proto == "" {
		w.proto = "HTTP/1.1"
	}

	statusText := http.StatusText(w.statusCode)
	if statusText == "" {
		statusText = "status code " + strconv.Itoa(w.statusCode)
	}

	// Format HTTP Status Line: HTTP/1.1 200 OK\r\n
	fmt.Fprintf(w.bufWriter, "%s %d %s\r\n", w.proto, w.statusCode, statusText)

	if w.closeConn {
		w.header.Set("Connection", "close")
	}

	// Format Field Lines: Field-Name: Field-Value\r\n
	for k, vals := range w.header {
		for _, v := range vals {
			fmt.Fprintf(w.bufWriter, "%s: %s\r\n", k, v)
		}
	}
	w.bufWriter.WriteString("\r\n")
}

func (w *responseWriter) finish() error {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.bufWriter.Flush()
}
