package http1

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ParseRequest parses an HTTP/1.1 stream into an *http.Request.
// It returns an *HTTPError containing RFC-compliant status codes (e.g. 400, 505) on failure.
func ParseRequest(reader *bufio.Reader) (*http.Request, error) {
	p := &requestParser{
		reader: reader,
		state:  stateRequestLine,
		req: &http.Request{
			Header: make(http.Header),
		},
	}
	return p.parse()
}

// HTTPError attaches an RFC-compliant HTTP status code to parser failures.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http1: %d %s: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Message)
}

func badRequest(msg string) error {
	return &HTTPError{StatusCode: http.StatusBadRequest, Message: msg}
}

func versionNotSupported(msg string) error {
	return &HTTPError{StatusCode: http.StatusHTTPVersionNotSupported, Message: msg}
}

func notImplemented(msg string) error {
	return &HTTPError{StatusCode: http.StatusNotImplemented, Message: msg}
}

type parserState int

const (
	stateRequestLine parserState = iota
	stateHeaders
	stateBody
	stateComplete
)

type requestParser struct {
	reader *bufio.Reader
	state  parserState
	req    *http.Request
}

func (p *requestParser) parse() (*http.Request, error) {
	for p.state != stateComplete {
		switch p.state {
		case stateRequestLine:
			line, err := p.readLine()
			if err != nil {
				return nil, err
			}
			if len(line) == 0 {
				continue // RFC 9112 §2.2: Ignore empty lines before request-line
			}
			if err := p.parseRequestLine(line); err != nil {
				return nil, err
			}
			p.state = stateHeaders

		case stateHeaders:
			line, err := p.readLine()
			if err != nil {
				return nil, err
			}
			if len(line) == 0 {
				p.state = stateBody
				continue // RFC 9112 §2.1: Empty line terminates headers
			}
			if err := p.parseFieldLine(line); err != nil {
				return nil, err
			}

		case stateBody:
			if err := p.setupBodyReader(); err != nil {
				return nil, err
			}
			p.state = stateComplete
		}
	}

	return p.req, nil
}

func (p *requestParser) parseRequestLine(line []byte) error {
	parts := bytes.Split(line, []byte(" "))
	if len(parts) != 3 || len(parts[0]) == 0 || len(parts[1]) == 0 || len(parts[2]) == 0 {
		return badRequest("malformed request-line")
	}

	method, target, version := parts[0], parts[1], parts[2]

	if err := p.validateToken(method); err != nil {
		return badRequest("invalid method token")
	}
	p.req.Method = string(method)

	rawURL := string(target)
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return badRequest("invalid request-target URI")
	}
	p.req.URL = parsedURL
	p.req.RequestURI = rawURL

	major, minor, err := p.parseHTTPVersion(version)
	if err != nil {
		return err
	}
	p.req.ProtoMajor = major
	p.req.ProtoMinor = minor
	p.req.Proto = string(version)

	return nil
}

func (p *requestParser) parseFieldLine(line []byte) error {
	if line[0] == ' ' || line[0] == '\t' {
		return badRequest("obsolete line folding (obs-fold) is prohibited")
	}

	colonIdx := bytes.IndexByte(line, ':')
	if colonIdx <= 0 {
		return badRequest("malformed header line: missing colon")
	}

	fieldName := line[:colonIdx]
	if fieldName[len(fieldName)-1] == ' ' || fieldName[len(fieldName)-1] == '\t' {
		return badRequest("whitespace before colon is prohibited")
	}

	if err := p.validateToken(fieldName); err != nil {
		return badRequest("invalid character in field-name")
	}

	fieldVal := bytes.Trim(line[colonIdx+1:], " \t")
	for _, b := range fieldVal {
		if (b < 0x20 && b != '\t') || b == 0x7F {
			return badRequest("unprintable control characters in header value")
		}
	}

	nameStr := string(fieldName)
	valStr := string(fieldVal)

	if strings.EqualFold(nameStr, "Host") {
		p.req.Host = valStr
	}

	p.req.Header.Add(nameStr, valStr)
	return nil
}

func (p *requestParser) setupBodyReader() error {
	if p.req.ProtoMajor == 1 && p.req.ProtoMinor >= 1 {
		if p.req.Host == "" || len(p.req.Header.Values("Host")) != 1 {
			return badRequest("HTTP/1.1 requests require exactly one Host header")
		}
	}

	teHeaders := p.req.Header.Values("Transfer-Encoding")
	clHeaders := p.req.Header.Values("Content-Length")

	if len(teHeaders) > 0 && len(clHeaders) > 0 {
		return badRequest("request contains both Transfer-Encoding and Content-Length")
	}

	if len(teHeaders) > 0 {
		combinedTE := strings.Join(teHeaders, ",")
		codings := strings.Split(combinedTE, ",")
		lastCoding := strings.ToLower(strings.TrimSpace(codings[len(codings)-1]))

		if lastCoding != "chunked" {
			return notImplemented(fmt.Sprintf("unsupported transfer coding: %s", lastCoding))
		}

		p.req.Body = &chunkedReader{r: p.reader}
		p.req.ContentLength = -1
		return nil
	}

	if len(clHeaders) > 0 {
		var canonicalLength int64 = -1
		for _, rawCL := range clHeaders {
			for _, part := range strings.Split(rawCL, ",") {
				cleanVal := strings.TrimSpace(part)
				val, err := strconv.ParseInt(cleanVal, 10, 64)
				if err != nil || val < 0 {
					return badRequest("invalid Content-Length value")
				}
				if canonicalLength != -1 && canonicalLength != val {
					return badRequest("conflicting Content-Length header values")
				}
				canonicalLength = val
			}
		}

		p.req.ContentLength = canonicalLength
		if canonicalLength == 0 {
			p.req.Body = http.NoBody
		} else {
			p.req.Body = io.NopCloser(io.LimitReader(p.reader, canonicalLength))
		}
		return nil
	}

	p.req.Body = http.NoBody
	p.req.ContentLength = 0
	return nil
}

func (p *requestParser) parseHTTPVersion(v []byte) (int, int, error) {
	if len(v) != 8 || !bytes.HasPrefix(v, []byte("HTTP/")) || v[6] != '.' {
		return 0, 0, badRequest("malformed HTTP-version")
	}

	majorByte := v[5]
	minorByte := v[7]

	if majorByte < '0' || majorByte > '9' || minorByte < '0' || minorByte > '9' {
		return 0, 0, badRequest("HTTP-version numbers must be single digits")
	}

	major := int(majorByte - '0')
	minor := int(minorByte - '0')

	if major != 1 {
		return 0, 0, versionNotSupported(fmt.Sprintf("unsupported major HTTP version: %d", major))
	}

	return major, minor, nil
}

func (p *requestParser) validateToken(token []byte) error {
	if len(token) == 0 {
		return badRequest("empty token")
	}
	for _, b := range token {
		if !p.isTChar(b) {
			return badRequest(fmt.Sprintf("invalid character in token: 0x%02x", b))
		}
	}
	return nil
}

func (p *requestParser) isTChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		bytes.IndexByte([]byte("!#$%&'*+-.^_`|~"), b) != -1
}

func (p *requestParser) readLine() ([]byte, error) {
	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF && len(line) == 0 {
			return nil, io.EOF
		}
		if err == io.EOF {
			return nil, badRequest("unexpected EOF before line completed")
		}
		return nil, err
	}

	if bytes.HasSuffix(line, []byte("\r\n")) {
		line = line[:len(line)-2]
	} else if bytes.HasSuffix(line, []byte("\n")) {
		line = line[:len(line)-1]
	}

	if bytes.IndexByte(line, '\r') != -1 {
		return nil, badRequest("bare CR character in input line is forbidden")
	}

	return line, nil
}

type chunkedReader struct {
	r         *bufio.Reader
	remaining int64
	done      bool
}

func (cr *chunkedReader) Read(p []byte) (int, error) {
	if cr.done {
		return 0, io.EOF
	}

	if cr.remaining == 0 {
		line, err := cr.r.ReadBytes('\n')
		if err != nil {
			return 0, badRequest("malformed chunk header")
		}

		line = bytes.TrimRight(line, "\r\n")
		if semi := bytes.IndexByte(line, ';'); semi != -1 {
			line = line[:semi]
		}
		line = bytes.TrimSpace(line)

		size, err := strconv.ParseInt(string(line), 16, 64)
		if err != nil || size < 0 {
			return 0, badRequest("invalid hexadecimal chunk size")
		}

		if size == 0 {
			cr.done = true
			for {
				trailerLine, err := cr.r.ReadBytes('\n')
				if err != nil {
					return 0, badRequest("malformed chunk trailers")
				}
				if len(bytes.TrimRight(trailerLine, "\r\n")) == 0 {
					break
				}
			}
			return 0, io.EOF
		}

		cr.remaining = size
	}

	toRead := int64(len(p))
	if toRead > cr.remaining {
		toRead = cr.remaining
	}

	n, err := cr.r.Read(p[:toRead])
	cr.remaining -= int64(n)

	if cr.remaining == 0 {
		crlf := make([]byte, 2)
		if _, err := io.ReadFull(cr.r, crlf); err != nil || crlf[0] != '\r' || crlf[1] != '\n' {
			return n, badRequest("missing CRLF after chunk data")
		}
	}

	return n, err
}

func (cr *chunkedReader) Close() error {
	cr.done = true
	return nil
}
