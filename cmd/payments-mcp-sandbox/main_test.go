package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunWithoutArgumentsIsInert(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() output = %q, want no output", stdout.String())
	}
}

func TestRunPrintsBuildMetadata(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-version"}, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	for _, field := range []string{
		"payments-mcp-sandbox",
		"version=",
		"commit=",
		"go=go",
	} {
		if !strings.Contains(stdout.String(), field) {
			t.Errorf("run() output %q does not contain %q", stdout.String(), field)
		}
	}
}

func TestRunReturnsWriterError(t *testing.T) {
	wantErr := errors.New("write failed")
	err := run([]string{"-version"}, errorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want wrapped %v", err, wantErr)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
