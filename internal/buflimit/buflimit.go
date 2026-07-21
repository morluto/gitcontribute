// Package buflimit provides bounded in-memory command output capture.
package buflimit

import (
	"bytes"
	"errors"
)

// ErrOutputLimit reports that a Buffer retained its byte limit and rejected
// the remainder of a write.
var ErrOutputLimit = errors.New("output exceeds limit")

// Buffer retains at most a fixed number of bytes.
type Buffer struct {
	buf       bytes.Buffer
	remaining int
	exceeded  bool
	err       error
}

// NewBuffer returns an empty Buffer with a non-negative byte limit.
func NewBuffer(limit int) *Buffer {
	if limit < 0 {
		limit = 0
	}
	return &Buffer{remaining: limit}
}

func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) <= b.remaining {
		n, err := b.buf.Write(p)
		b.remaining -= n
		return n, err
	}
	written := b.remaining
	if written > 0 {
		_, _ = b.buf.Write(p[:written])
		b.remaining = 0
	}
	b.exceeded = true
	b.err = ErrOutputLimit
	return written, ErrOutputLimit
}

func (b *Buffer) String() string { return b.buf.String() }

// Bytes returns the retained bytes.
func (b *Buffer) Bytes() []byte { return b.buf.Bytes() }

// Exceeded reports whether a write exceeded the configured limit.
func (b *Buffer) Exceeded() bool { return b.exceeded }

// Err returns the output-limit error recorded by Write, if any.
func (b *Buffer) Err() error { return b.err }
