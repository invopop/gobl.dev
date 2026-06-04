package bundle

// Package bundle declares the addon set that GOBL.dev ships with. Blank-importing
// it registers every addon listed here, so both the CLI and the web/API binaries
// support the same set. Add a blank import per approved addon module — this is the
// one place to update.
import (
	_ "github.com/invopop/gobl/addons" // all in-core addons
	// External addons are added here as they're approved, e.g.:
	// _ "github.com/invopop/gobl.fr.ctc/addon"
)
