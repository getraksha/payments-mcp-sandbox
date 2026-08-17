package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrent(t *testing.T) {
	originalVersion, originalCommit := version, commit
	t.Cleanup(func() {
		version, commit = originalVersion, originalCommit
	})

	version = "v1.2.3"
	commit = "0123abcd"

	got := Current()
	if got.Service != ServiceName {
		t.Fatalf("Service = %q, want %q", got.Service, ServiceName)
	}
	if got.Version != version {
		t.Fatalf("Version = %q, want %q", got.Version, version)
	}
	if got.Commit != commit {
		t.Fatalf("Commit = %q, want %q", got.Commit, commit)
	}
	if got.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
	}
}
