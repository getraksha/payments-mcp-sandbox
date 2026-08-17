// Package buildinfo exposes non-sensitive metadata about the service binary.
package buildinfo

import "runtime"

const ServiceName = "payments-mcp-sandbox"

// version and commit are set at build time with -ldflags. The defaults make
// local, unversioned builds explicit instead of presenting misleading values.
var (
	version = "dev"
	commit  = "unknown"
)

// Info is the safe build metadata that later startup logs and diagnostics may
// expose.
type Info struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
}

// Current returns metadata for the running binary.
func Current() Info {
	return Info{
		Service:   ServiceName,
		Version:   version,
		Commit:    commit,
		GoVersion: runtime.Version(),
	}
}
