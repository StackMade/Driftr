package ioutil

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestProgressWriter_CountsBytes(t *testing.T) {
	var buf bytes.Buffer
	pw := &ProgressWriter{Dest: &buf, Total: 1000}

	data := make([]byte, 500)
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != 500 {
		t.Errorf("Write() returned %d, want 500", n)
	}

	n, err = pw.Write(data[:300])
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != 300 {
		t.Errorf("Write() returned %d, want 300", n)
	}
}

func TestProgressWriter_DelegatesToDest(t *testing.T) {
	var buf bytes.Buffer
	pw := &ProgressWriter{Dest: &buf, Total: -1}

	data := []byte("hello driftr")
	if _, err := pw.Write(data); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	if got := buf.String(); got != "hello driftr" {
		t.Errorf("dest received %q, want %q", got, "hello driftr")
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestProgressWriter_PropagatesErrors(t *testing.T) {
	pw := &ProgressWriter{Dest: errWriter{}, Total: 100}

	_, err := pw.Write([]byte("data"))
	if err == nil {
		t.Fatal("expected error from Write(), got nil")
	}
	if err.Error() != "disk full" {
		t.Errorf("error = %q, want %q", err.Error(), "disk full")
	}
}

func TestIsTerminal_Pipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if IsTerminal(r) {
		t.Error("IsTerminal(pipe reader) = true, want false")
	}
	if IsTerminal(w) {
		t.Error("IsTerminal(pipe writer) = true, want false")
	}
}

// captureStderr runs fn and returns everything it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestProgressWriter_Finish(t *testing.T) {
	tests := []struct {
		name  string
		total int64
		want  string
	}{
		// Content-Length known: both halves of the ratio are printed.
		{"known total", 4 * 1024 * 1024, "\r  Downloading: 2.0 MB / 4.0 MB\n"},
		// Unknown length (-1) or a zero total: only what was downloaded.
		{"unknown total", -1, "\r  Downloading: 2.0 MB\n"},
		{"zero total", 0, "\r  Downloading: 2.0 MB\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pw := &ProgressWriter{Dest: io.Discard, Total: tt.total}
			if _, err := pw.Write(make([]byte, 2*1024*1024)); err != nil {
				t.Fatal(err)
			}
			got := captureStderr(t, pw.Finish)
			// Write may have printed a progress line of its own; Finish's
			// output is the last one, and it is the one that ends in a newline.
			if !strings.HasSuffix(got, tt.want) {
				t.Errorf("Finish() wrote %q, want it to end with %q", got, tt.want)
			}
		})
	}
}
