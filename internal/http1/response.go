package http1

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

// Response implements http.ResponseWriter for serializing HTTP/1.1 wire responses.
type Response struct {
	conn        net.Conn
	bufWriter   *bufio.Writer
	header      http.Header
	wroteHeader bool
	statusCode  int
	Proto       string
	CloseConn   bool
}

// NewResponse initializes a response wrapper around an active net.Conn socket.
func NewResponse(conn net.Conn) *Response {
	return &Response{
		conn:       conn,
		bufWriter:  bufio.NewWriter(conn),
		header:     make(http.Header),
		Proto:      "HTTP/1.1",
		statusCode: http.StatusOK,
	}
}

// Header returns the header map that will be sent by WriteHeader.
func (r *Response) Header() http.Header {
	return r.header
}

// WriteHeader sends an HTTP response header with the provided status code.
func (r *Response) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = statusCode
	r.flushHeader()
}

// Write writes the data to the connection as part of an HTTP reply.
// If WriteHeader has not yet been called, Write calls WriteHeader(http.StatusOK).
func (r *Response) Write(data []byte) (n int, err error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.bufWriter.Write(data)
}

// flushHeader serializes the status-line and field lines to the buffered writer.
func (r *Response) flushHeader() {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true

	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	if r.Proto == "" {
		r.Proto = "HTTP/1.1"
	}

	statusText := http.StatusText(r.statusCode)
	if statusText == "" {
		statusText = "status code " + strconv.Itoa(r.statusCode)
	}

	// RFC 9112 §3.1: status-line = HTTP-version SP status-code SP [reason-phrase] CRLF
	fmt.Fprintf(r.bufWriter, "%s %d %s\r\n", r.Proto, r.statusCode, statusText)

	if r.CloseConn {
		r.header.Set("Connection", "close")
	}

	// RFC 9112 §5: field-line = field-name ":" OWS field-value OWS CRLF
	for key, values := range r.header {
		for _, value := range values {
			fmt.Fprintf(r.bufWriter, "%s: %s\r\n", key, value)
		}
	}
	r.bufWriter.WriteString("\r\n")
}

// Finish flushes any buffered data directly to the underlying TCP connection.
// It is called by the server loop when the handler completes execution.
func (r *Response) Finish() error {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.bufWriter.Flush()
}
