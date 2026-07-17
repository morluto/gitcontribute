package discovery

import (
	"errors"
	"io"
)

// ErrLimitExceeded is the generic size-limit error returned by LimitedReader.
var ErrLimitExceeded = errors.New("size limit exceeded")

// LimitedReader caps the number of bytes read from an underlying reader and
// returns a configured error if the limit is exceeded. It closes the
// underlying read-closer when closed.
type LimitedReader struct {
	R         io.Reader
	N         int64
	Err       error
	CloseFunc func() error
	exceeded  bool
	eof       bool
}

// NewLimitedReader returns a reader that returns err after reading more than n
// bytes. If err is nil, ErrLimitExceeded is used. The close function is called
// on Close; if nil, Close is a no-op unless R implements io.ReadCloser.
func NewLimitedReader(r io.Reader, n int64, closeFunc func() error, err error) *LimitedReader {
	if err == nil {
		err = ErrLimitExceeded
	}
	return &LimitedReader{R: r, N: n, CloseFunc: closeFunc, Err: err}
}

// Read returns up to the remaining allowed bytes. When the limit is reached,
// the next call returns Err. It returns io.EOF if the underlying stream ends
// exactly at the limit.
func (l *LimitedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if l.exceeded {
		return 0, l.Err
	}
	if l.eof {
		return 0, io.EOF
	}
	if l.N < 0 {
		return 0, l.Err
	}
	if l.N == 0 {
		var probe [1]byte
		n, err := l.R.Read(probe[:])
		if n > 0 {
			l.exceeded = true
			return 0, l.Err
		}
		return 0, err
	}
	if int64(len(p)) > l.N {
		p = p[:l.N]
	}
	n, err := l.R.Read(p)
	l.N -= int64(n)
	if err == io.EOF && n > 0 {
		l.eof = true
		return n, nil
	}
	return n, err
}

// Close releases the underlying reader.
func (l *LimitedReader) Close() error {
	if l.CloseFunc != nil {
		return l.CloseFunc()
	}
	if rc, ok := l.R.(io.ReadCloser); ok {
		return rc.Close()
	}
	return nil
}
