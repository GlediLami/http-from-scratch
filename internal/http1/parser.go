package http1

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var errLineTooLong = errors.New("line too long")

func ParseRequest(r *bufio.Reader) (*http.Request, error) {
	line, err := readStartLine(r)
	if err != nil {
		return nil, err
	}
	method, target, proto, err := parseRequestLine(line)

	if err != nil {
		return nil, err
	}
	major, minor, err := parseVersion(proto)
	if err != nil {
		return nil, err
	}

	header, err := readHeaders(r)
	if err != nil {
		return nil, err
	}
	req := &http.Request{
		Method:     method,
		RequestURI: target,
		Proto:      proto,
		ProtoMajor: major,
		ProtoMinor: minor,
		Header:     header,
	}
	if err := setHost(req, target); err != nil {
		return nil, err
	}
	if err := setURL(req, target); err != nil {
		return nil, err
	}
	if err := setBody(req, r); err != nil {
		return nil, err
	}
	req.Close = shouldClose(req)
	return req, nil
}

// reads one CRLF terminated line and returns it without the CRLF
// a LF without a CR is rejected
// ReadSlice('\n') hands back to internal buffer so we copy with string(...)
// err buffer full gives us a free length limit
func readLine(r *bufio.Reader, limit int) (string, error) {
	var buf []byte
	for {

		slice, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			buf = append(buf, slice...)

			if len(buf) > limit {
				return "", errLineTooLong
			}

			continue
		}

		if err != nil {
			if err == io.EOF && len(buf) == 0 && len(slice) == 0 {
				return "", io.EOF
			}
			if err == io.EOF {
				return "", protoErr(http.StatusBadRequest, "line truncated by EOF")
			}
			return "", err
		}

		buf = append(buf, slice...)
		break

	}

	if len(buf) > limit {
		return "", errLineTooLong
	}

	n := len(buf)

	if n < 2 || buf[n-1] != '\n' || buf[n-2] != '\r' {
		return "", protoErr(http.StatusBadRequest, "line not terminated by CRLF")
	}

	return string(buf[:n-2]), nil
}

// returns first non empty line
func readStartLine(r *bufio.Reader) (string, error) {
	for tries := 0; tries < 2; tries++ {
		line, err := readLine(r, maxRequestLineBytes)
		if errors.Is(err, errLineTooLong) {
			return "", protoErr(http.StatusRequestURITooLong, "request line too long ")
		}
		if err != nil {
			return "", err
		}
		if line != "" {
			return line, nil
		}
	}
	return "", protoErr(http.StatusBadRequest, "too many blank lines before request")
}

// Grammar -> method SP request-target SP HTTP-version . 1 space between parts
// 1st and last space and reject any space left inside
// Get / with double space fails
func parseRequestLine(line string) (method, target, proto string, err error) {
	first := strings.IndexByte(line, ' ')
	last := strings.LastIndexByte(line, ' ')

	if first <= 0 || first == last {
		return "", "", "", protoErr(http.StatusBadRequest, "malformed request line?")
	}

	method = line[:first]
	target = line[first+1 : last]
	proto = line[last+1:]
	if strings.IndexByte(target, ' ') != -1 {
		return "", "", "", protoErr(http.StatusBadRequest, "extra space in request line")
	}
	if !isToken(method) {
		return "", "", "", protoErr(http.StatusBadRequest, "invalid method")
	}

	if target == "" {
		return "", "", "", protoErr(http.StatusBadRequest, "empty request target")
	}
	return method, target, proto, nil
}

func parseVersion(proto string) (major, minor int, err error) {

	rest, ok := strings.CutPrefix(proto, "HTTP/")

	if !ok {
		return 0, 0, protoErr(http.StatusBadRequest, "malformed HTTP version")
	}

	hi, lo, ok := strings.Cut(rest, ".")

	if !ok || !isDigits(hi) || !isDigits(lo) {
		return 0, 0, protoErr(http.StatusBadRequest, "malformed HTTP version")
	}

	if hi != "1" || (lo != "0" && lo != "1") {
		return 0, 0, protoErr(http.StatusHTTPVersionNotSupported, "unsupported http version")
	}

	return 1, int(lo[0] - '0'), nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func readHeaders(r *bufio.Reader) (http.Header, error) {
	h := make(http.Header)
	for n := 0; ; n++ {
		line, err := readLine(r, maxHeaderLineBytes)
		if err == io.EOF {
			return nil, protoErr(http.StatusBadRequest, "bad request")
		}
		if errors.Is(err, errLineTooLong) {
			return nil, protoErr(http.StatusRequestHeaderFieldsTooLarge, "header line too long")
		}
		if err != nil {
			return nil, err
		}

		if line == "" {
			return h, nil
		}

		if n >= maxHeaderLines {
			return nil, protoErr(http.StatusRequestHeaderFieldsTooLarge, "too many header fields")
		}

		if line[0] == ' ' || line[0] == '\t' {
			return nil, protoErr(http.StatusBadRequest, "obsolete line folding?")
		}

		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			return nil, protoErr(http.StatusBadRequest, "malformed header field")
		}
		name := line[:colon]
		if !isToken(name) {
			return nil, protoErr(http.StatusBadRequest, "invalid header name")
		}
		value := strings.Trim(line[colon+1:], " \t")
		if !validFieldValue(value) {
			return nil, protoErr(http.StatusBadRequest, "invalid header value")
		}
		h.Add(name, value)
	}
}

func setHost(req *http.Request, target string) error {
	hosts := req.Header["Host"]
	if len(hosts) > 1 {
		return protoErr(http.StatusBadRequest, "multiple Host Headers")
	}
	if req.ProtoMajor == 1 && req.ProtoMinor == 1 && len(hosts) != 1 {
		return protoErr(http.StatusBadRequest, "missing Host Header")
	}
	if len(hosts) == 1 {
		req.Host = hosts[0]
	}

	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			req.Host = u.Host
		}
	}
	req.Header.Del("Host")
	return nil
}

func setURL(req *http.Request, target string) error {
	if target == "*" {
		req.URL = &url.URL{Path: "*"}
		return nil
	}
	u, err := url.ParseRequestURI(target)
	if err != nil {
		return protoErr(http.StatusBadRequest, "invalid request target")
	}

	req.URL = u
	return nil
}

// Content-Length and Transfer-Encoding together is a 400
// chunked must be the only coding
// A Content-Length body is read with a bounded LimitReader
// No framing headers means a request has no body
func setBody(req *http.Request, r *bufio.Reader) error {
	te := req.Header.Values("Transfer-Encoding")
	cl := req.Header.Values("Content-Length")

	if len(te) > 0 && len(cl) > 0 {
		return protoErr(http.StatusBadRequest, "both Transfer-Encoding and Content-Length")
	}

	if len(te) > 0 {
		if req.ProtoMajor == 1 && req.ProtoMinor == 0 {
			return protoErr(http.StatusBadRequest, "Transfer-Encoding in HTTP/1.0 ?")
		}

		codings := splitList(te)
		if len(codings) != 1 || codings[0] != "chunked" {
			return protoErr(http.StatusBadRequest, "unsupported transfer coding")
		}
		req.TransferEncoding = []string{"chunked"}
		req.ContentLength = -1
		req.Header.Del("Transfer-Encoding")
		req.Body = &body{src: newChunkedReader(r), expect: -1}
		return nil
	}

	if len(cl) > 0 {
		n, ok := parseContentLength(cl)
		if !ok {
			return protoErr(http.StatusBadRequest, "invalid Content-Length")
		}
		if n > maxBodyBytes {
			return protoErr(http.StatusBadRequest, "body too large")
		}
		req.ContentLength = n
		if n == 0 {
			req.Body = http.NoBody
			return nil
		}
		req.Body = &body{src: io.LimitReader(r, n), expect: n}
		return nil
	}

	req.ContentLength = 0
	req.Body = http.NoBody
	return nil
}

func splitList(values []string) []string {
	var out []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Content-Length must be only digits
// strconv.ParseInt alone accepts +5 and -5, so we check every byte first
// Repeated identical values are fine; different values are an attack, reject them
func parseContentLength(values []string) (int64, bool) {
	var parts []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			parts = append(parts, strings.TrimSpace(p))
		}
	}
	if len(parts) == 0 {
		return 0, false
	}
	first := parts[0]
	for _, p := range parts {
		if p != first {
			return 0, false
		}
	}
	if first == "" {
		return 0, false
	}
	for i := 0; i < len(first); i++ {
		if first[i] < '0' || first[i] > '9' {
			return 0, false
		}
	}

	n, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// shouldClose reads the keep-alive rules: HTTP/1.1 stays open unless Connection: close;
// HTTP/1.0 closes unless Connection: keep-alive.
func shouldClose(req *http.Request) bool {
	conn := map[string]bool{}
	for _, tok := range splitList(req.Header.Values("Connection")) {
		conn[tok] = true
	}
	if req.ProtoMajor == 1 && req.ProtoMinor == 1 {
		return conn["close"]
	}
	return !conn["keep-alive"]
}
