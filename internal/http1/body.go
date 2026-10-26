package http1

import "io"

type body struct {
	src    io.Reader
	expect int64
	read   int64
	eof    bool
}

func (b *body) Read(p []byte) (int, error) {
	if b.eof {
		return 0, io.EOF
	}
	n, err := b.src.Read(p)
	b.read += int64(n)
	if err == io.EOF {
		b.eof = true
		if b.expect >= 0 && b.read < b.expect {
			return n, io.ErrUnexpectedEOF
		}
	}
	return n, err
}

func (b *body) Close() error { return nil }
