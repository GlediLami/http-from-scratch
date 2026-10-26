package http1

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	headerTimeout = 10 * time.Second
	writeTimeout  = 30 * time.Second
	idleTimeout   = 60 * time.Second
)

func ListenAndServe(addr string, handler http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return Serve(ln, handler)
}

func Serve(ln net.Listener, handler http.Handler) error {
	defer ln.Close()
	if handler == nil {
		handler = http.DefaultServeMux
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go func() {
			if err := HandleConnection(conn, handler); err != nil {
				log.Printf("connection %s: %v", conn.RemoteAddr(), err)
			}
		}()
	}
}

// HandleConnection runs the request loop for one connection.
func HandleConnection(conn net.Conn, mux http.Handler) error {
	defer conn.Close()
	br := bufio.NewReader(conn)

	for {
		conn.SetReadDeadline(time.Now().Add(headerTimeout))

		req, perr := ParseRequest(br)
		if perr != nil {
			if errors.Is(perr, io.EOF) {
				return nil // client closed cleanly between requests
			}
			writeError(conn, perr)
			return nil
		}

		req.RemoteAddr = conn.RemoteAddr().String()
		ctx, cancel := context.WithCancel(context.Background())
		req = req.WithContext(ctx)

		conn.SetWriteDeadline(time.Now().Add(writeTimeout))

		w := NewResponse(conn)
		w.req = req
		w.closeAfter = req.Close

		serve(mux, w, req)

		if err := w.finish(); err != nil {
			cancel()
			return err
		}
		cancel()

		if !drain(req.Body) {
			return nil // unread body too large to drain: close
		}
		if w.closeAfter {
			return nil
		}
		conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}
}

func serve(mux http.Handler, w *Response, req *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("handler panic: %v", rec)
			if !w.wroteHeader {
				w.WriteHeader(http.StatusInternalServerError)
			}
			w.closeAfter = true
		}
	}()
	mux.ServeHTTP(w, req)
}

func writeError(conn net.Conn, perr error) {
	code := http.StatusBadRequest
	var pe *protocolError
	if errors.As(perr, &pe) {
		code = pe.code
	}
	bw := bufio.NewWriter(conn)
	fmt.Fprintf(bw, "HTTP/1.1 %d %s\r\n", code, http.StatusText(code))
	fmt.Fprintf(bw, "Content-Length: 0\r\n")
	fmt.Fprintf(bw, "Connection: close\r\n")
	fmt.Fprintf(bw, "Date: %s\r\n\r\n", time.Now().UTC().Format(http.TimeFormat))
	bw.Flush()
}

func drain(b io.ReadCloser) bool {
	if b == nil || b == http.NoBody {
		return true
	}
	n, _ := io.CopyN(io.Discard, b, maxDrainBytes+1)
	b.Close()
	return n <= maxDrainBytes
}
