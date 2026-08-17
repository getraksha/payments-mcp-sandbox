// payments-mcp-sandbox is a synthetic-only payment service. The Task 1
// executable intentionally serves no MCP or diagnostics requests.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/getraksha/payments-mcp-sandbox/internal/buildinfo"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(buildinfo.ServiceName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	showVersion := flags.Bool("version", false, "print build metadata and exit")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if !*showVersion {
		return nil
	}

	info := buildinfo.Current()
	_, err := fmt.Fprintf(
		stdout,
		"%s version=%s commit=%s go=%s\n",
		info.Service,
		info.Version,
		info.Commit,
		info.GoVersion,
	)
	if err != nil {
		return fmt.Errorf("write build metadata: %w", err)
	}
	return nil
}
