package http1

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

type Response struct {
	conn        net.Conn
	bw          *bufio.Writer
	req         *http.Request
	header      http.Header
	status      int
	wroteHeader bool
	buf         bytes.Buffer
	closeAfter  bool // set by server loop
}

var _ http.ResponseWriter = (*Response)(nil)

func NewResponse(conn net.Conn) *Response {
	return &Response{
		conn:   conn,
		bw:     bufio.NewWriter(conn),
		header: make(http.Header),
	}
}

func (r *Response) Header() http.Header { return r.header }

func (r *Response) WriteHeader(status int) {
	if r.wroteHeader {
		return // the first call wins
	}
	if status < 100 || status > 999 {
		status = http.StatusInternalServerError
	}
	r.status = status
	r.wroteHeader = true
}

func (r *Response) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.buf.Write(p)
}

// finish serializes the response onto the connection.
// status line, then headers, then a blank line, then the body
// An origin server must send Date
// 204 and 304 carry no body and no length headers
func (r *Response) finish() error {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	isHead := r.req.Method == http.MethodHead
	bodyAllowed := bodyAllowedForStatus(r.status)
	body := r.buf.Bytes()

	fmt.Fprintf(r.bw, "HTTP/1.1 %d %s\r\n", r.status, http.StatusText(r.status))

	if r.header.Get("Date") == "" {
		r.header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
	if bodyAllowed {
		r.header.Set("Content-Length", strconv.Itoa(len(body)))
	} else {
		r.header.Del("Content-Length")
		r.header.Del("Transfer-Encoding")
	}
	if r.closeAfter {
		r.header.Set("Connection", "close")
	}
	if err := writeHeaders(r.bw, r.header); err != nil {
		return err
	}
	if _, err := r.bw.WriteString("\r\n"); err != nil {
		return err
	}
	if bodyAllowed && !isHead {
		if _, err := r.bw.Write(body); err != nil {
			return err
		}
	}
	return r.bw.Flush()
}

func writeHeaders(bw *bufio.Writer, h http.Header) error {
	for name, values := range h {
		if !isToken(name) {
			continue
		}
		for _, v := range values {
			if !validFieldValue(v) { // block header injection
				continue
			}
			if _, err := fmt.Fprintf(bw, "%s: %s\r\n", name, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status < 200:
		return false
	case status == http.StatusNoContent, status == http.StatusNotModified:
		return false
	}
	return true
}
