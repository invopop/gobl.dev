// Package main provides a command-line interface to the GOBL library.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/invopop/gobl"

	// Register the full GOBL addon set (see bundle/bundle.go).
	_ "github.com/invopop/gobl.dev/bundle"
)

// build data provided by goreleaser and mage setup
var (
	version = "dev"
	date    = ""
)

func main() {
	if err := run(); err != nil {
		printError(err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return root().cmd().ExecuteContext(ctx)
}

func inputFilename(args []string) string {
	if len(args) > 0 && args[0] != "-" {
		return args[0]
	}
	return ""
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := struct {
				Version string `json:"version"`        // this gobl.dev / CLI release
				Core    string `json:"core"`           // the github.com/invopop/gobl it was built against
				Date    string `json:"date,omitempty"` // build date
			}{
				Version: cliVersion(),
				Core:    coreVersion(),
				Date:    date,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "\t") // always indent version
			return enc.Encode(out)
		},
	}
}

// cliVersion returns the gobl.dev CLI release: the ldflags value when built by
// mage/goreleaser, otherwise the module version recorded in the build info.
func cliVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

// coreVersion returns the version of github.com/invopop/gobl this binary was
// built against, taken from the build info (honouring any replace), and falling
// back to the version baked into the core library.
func coreVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path != "github.com/invopop/gobl" {
				continue
			}
			if d.Replace != nil && d.Replace.Version != "" {
				return d.Replace.Version
			}
			if d.Version != "" && d.Version != "(devel)" {
				return d.Version
			}
		}
	}
	return string(gobl.VERSION)
}

func encode(in any, out io.WriteCloser, indent bool) error {
	enc := json.NewEncoder(out)
	if indent {
		enc.SetIndent("", "\t")
	}
	return enc.Encode(in)
}

// newLogger builds the slog.Logger used for all operator-facing log
// output. The result writes one entry per line to stderr; result
// output (signed envelopes, /who party JSON, version) lives on stdout
// and is not affected by this flag.
func newLogger(jsonMode bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler
	if jsonMode {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

func printError(err error) {
	// Normalise to a *gobl.Error so every report carries a "key" and
	// (when present) a "message" + structured faults.
	var ge *gobl.Error
	if !errors.As(err, &ge) {
		ge = gobl.ErrInternal.WithCause(err)
	}
	attrs := []any{"key", ge.Key().String()}
	if msg := ge.Message(); msg != "" {
		attrs = append(attrs, "message", msg)
	}
	if faults := ge.Faults(); faults != nil {
		attrs = append(attrs, "faults", faults)
	}
	slog.Error("command failed", attrs...)
}
