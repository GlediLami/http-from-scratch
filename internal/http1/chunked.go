package http1

import (
	"bufio"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// chunked framing has no up-front length.
// It is a stream of size CRLF data CRLF blocks ending in a 0 chunk.
// We track remaining bytes in the current chunk and a sticky err so one framing mistake stops
// the stream for good.
type chunkedReader struct {
	r         *bufio.Reader
	remaining int64
	done      bool
	err       error
}

func newChunkedReader(r *bufio.Reader) *chunkedReader { return &chunkedReader{r: r} }

// when the current chunk is empty we read the next size line
// We never read past the chunk boundary (p = p[:c.remaining])
// then enforce the mandatory CRLF that follows the data
// A lax reader that skips that CRLF check is a smuggling vector.
func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.done {
		return 0, io.EOF
	}
	if c.remaining == 0 {
		if err := c.beginChunk(); err != nil {
			c.err = err
			return 0, err
		}
		if c.done {
			return 0, io.EOF
		}
	}

	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := io.ReadFull(c.r, p)
	c.remaining -= int64(n)
	if err != nil {
		c.err = err
		return n, err
	}

	if c.remaining == 0 {
		if err := c.expectCRLF(); err != nil {
			c.err = err
			return n, err
		}
	}
	return n, nil
}

func (c *chunkedReader) beginChunk() error {
	line, err := readLine(c.r, maxHeaderLineBytes)
	if err != nil {
		return err
	}

	if i := strings.IndexByte(line, ';'); i >= 0 {
		// drop line extensions
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	size, err := strconv.ParseInt(line, 16, 64)
	if err != nil || size == 0 || size > maxChunkSize {
		return protoErr(http.StatusBadRequest, "invalid chunk size")
	}
	if size == 0 {
		for {
			t, err := readLine(c.r, maxHeaderLineBytes)
			if err != nil {
				return err
			}
			if t == "" {
				break
			}
		}
		c.done = true
		return nil
	}
	c.remaining = size
	return nil
}

// the size is hex. We cap it at maxChunkSize so ffffffff... cannot overflow
// Chunk extensions (;name=val) are attacker bytes we ignore but bound by the line limit (
func (c *chunkedReader) expectCRLF() error {
	var b [2]byte
	if _, err := io.ReadFull(c.r, b[:]); err != nil {
		return err
	}
	if b[0] != '\r' || b[1] != '\n' {
		return protoErr(http.StatusBadRequest, "missing CRLF after chunk data")
	}
	return nil
}
