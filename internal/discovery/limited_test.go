package discovery

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLimitedReaderAllowsExactLimit(t *testing.T) {
	r := NewLimitedReader(strings.NewReader("four"), 4, nil, nil)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "four" {
		t.Fatalf("body = %q, want four", got)
	}
}

func TestLimitedReaderRejectsBytePastLimit(t *testing.T) {
	r := NewLimitedReader(strings.NewReader("five!"), 4, nil, nil)
	got, err := io.ReadAll(r)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("error = %v, want ErrLimitExceeded", err)
	}
	if string(got) != "five" {
		t.Fatalf("body = %q, want five", got)
	}
}
