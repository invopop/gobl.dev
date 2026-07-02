package bundle

// Package bundle declares the addon set that GOBL.dev ships with. Blank-importing
// it registers every addon listed here, so both the CLI and the web/API binaries
// support the same set. Add a blank import per approved addon module — this is the
// one place to update.
import (
	_ "github.com/invopop/gobl.br.nfe/addon"
	_ "github.com/invopop/gobl.br.nfse/addon"
	_ "github.com/invopop/gobl.fr.ctc/addon"
	_ "github.com/invopop/gobl.mx.cfdi/addon"
	_ "github.com/invopop/gobl.pt.saft/addon"
	_ "github.com/invopop/gobl.sa.zatca/addon"
)
