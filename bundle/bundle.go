package bundle

// Package bundle declares the addon and converter set that GOBL.dev ships with.
// Blank-importing it registers every addon and converter listed here, so both the
// CLI and the web/API binaries support the same set. Add a blank import per
// approved addon module, or per converter module that registers formats with
// GOBL's convert package — this is the one place to update.
import (
	// Addons
	_ "github.com/invopop/gobl.ar.arca/addon"
	_ "github.com/invopop/gobl.br.nfe/addon"
	_ "github.com/invopop/gobl.br.nfse/addon"
	_ "github.com/invopop/gobl.co.dian/addon"
	_ "github.com/invopop/gobl.dk.oioubl/addon"
	_ "github.com/invopop/gobl.es.verifactu/addon"
	_ "github.com/invopop/gobl.fi.finvoice/addon"
	_ "github.com/invopop/gobl.fr.ctc/addon"
	_ "github.com/invopop/gobl.it.sdi/addon"
	_ "github.com/invopop/gobl.mx.cfdi/addon"
	_ "github.com/invopop/gobl.pl.ksef/addon"
	_ "github.com/invopop/gobl.pt.saft/addon"
	_ "github.com/invopop/gobl.sa.zatca/addon"

	// Converters
	_ "github.com/invopop/gobl.cii"
	_ "github.com/invopop/gobl.de.xinvoice"
	_ "github.com/invopop/gobl.es.facturae"
	_ "github.com/invopop/gobl.fr.ctc/cdar"
	_ "github.com/invopop/gobl.fr.ctc/cii"
	_ "github.com/invopop/gobl.fr.ctc/ubl"
	_ "github.com/invopop/gobl.sa.zatca/ubl"
	_ "github.com/invopop/gobl.ubl"
)
