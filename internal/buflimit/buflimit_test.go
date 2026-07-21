package buflimit

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestBufferCapturesUpToLimit(t *testing.T) {
	b := NewBuffer(5)
	n, err := b.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	if b.String() != "hello" || b.Exceeded() {
		t.Fatalf("after exact write: String=%q, Exceeded=%v", b.String(), b.Exceeded())
	}
	if b.Err() != nil {
		t.Fatalf("Err = %v, want nil", b.Err())
	}
}

func TestBufferTruncatesAndReportsLimit(t *testing.T) {
	b := NewBuffer(3)
	n, err := b.Write([]byte("hello"))
	if n != 3 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Write = (%d, %v), want (3, ErrOutputLimit)", n, err)
	}
	if b.String() != "hel" || !b.Exceeded() {
		t.Fatalf("after overflow: String=%q, Exceeded=%v", b.String(), b.Exceeded())
	}
	n, err = b.Write([]byte("more"))
	if n != 0 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("second Write = (%d, %v), want (0, ErrOutputLimit)", n, err)
	}
	if b.String() != "hel" {
		t.Fatalf("String after second write = %q, want %q", b.String(), "hel")
	}
}

func TestBufferMultipleWrites(t *testing.T) {
	b := NewBuffer(6)
	if _, err := b.Write([]byte("abc")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	n, err := b.Write([]byte("defghi"))
	if n != 3 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("second Write = (%d, %v), want (3, ErrOutputLimit)", n, err)
	}
	if b.String() != "abcdef" {
		t.Fatalf("String = %q, want %q", b.String(), "abcdef")
	}
}

func TestBufferZeroLimit(t *testing.T) {
	b := NewBuffer(0)
	n, err := b.Write([]byte("x"))
	if n != 0 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Write = (%d, %v), want (0, ErrOutputLimit)", n, err)
	}
	if b.String() != "" {
		t.Fatalf("String = %q, want empty", b.String())
	}
}

func TestBufferNegativeLimitCappedAtZero(t *testing.T) {
	b := NewBuffer(-1)
	n, err := b.Write([]byte("x"))
	if n != 0 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Write = (%d, %v), want (0, ErrOutputLimit)", n, err)
	}
}

func TestBufferBytes(t *testing.T) {
	b := NewBuffer(4)
	b.Write([]byte("abcd"))
	if !bytes.Equal(b.Bytes(), []byte("abcd")) {
		t.Fatalf("Bytes = %v, want %v", b.Bytes(), []byte("abcd"))
	}
}

var _ io.Writer = (*Buffer)(nil)
