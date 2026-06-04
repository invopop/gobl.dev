package editor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	goblapi "github.com/invopop/gobl/pkg/api"

	cii "github.com/invopop/gobl.cii"
	fatturapa "github.com/invopop/gobl.fatturapa"
	goblhtml "github.com/invopop/gobl.html"
	ubl "github.com/invopop/gobl.ubl"
)

// Format describes an output format offered by the viewer pane.
type Format struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	MIME  string `json:"mime"`

	convert func(ctx context.Context, env *gobl.Envelope) ([]byte, error)
}

var formats = []Format{
	{
		ID:      "ubl",
		Label:   "UBL — EN 16931",
		Group:   "UBL",
		MIME:    "application/xml",
		convert: convertUBL(ubl.ContextEN16931),
	},
	{
		ID:      "ubl-peppol",
		Label:   "UBL — Peppol BIS 3.0",
		Group:   "UBL",
		MIME:    "application/xml",
		convert: convertUBL(ubl.ContextPeppol),
	},
	{
		ID:      "ubl-xrechnung",
		Label:   "UBL — XRechnung",
		Group:   "UBL",
		MIME:    "application/xml",
		convert: convertUBL(ubl.ContextXRechnung),
	},
	{
		ID:      "ubl-peppol-fr-cius",
		Label:   "UBL — Peppol France CIUS",
		Group:   "UBL",
		MIME:    "application/xml",
		convert: convertUBL(ubl.ContextPeppolFranceCIUS),
	},
	{
		ID:      "ubl-peppol-fr-ext",
		Label:   "UBL — Peppol France Extended",
		Group:   "UBL",
		MIME:    "application/xml",
		convert: convertUBL(ubl.ContextPeppolFranceExtended),
	},
	{
		ID:    "cii",
		Label: "CII — EN 16931",
		Group: "CII",
		MIME:  "application/xml",
		convert: convertCII(cii.ContextEN16931V2017),
	},
	{
		ID:      "cii-peppol",
		Label:   "CII — Peppol BIS 3.0",
		Group:   "CII",
		MIME:    "application/xml",
		convert: convertCII(cii.ContextPeppolV3),
	},
	{
		ID:      "cii-facturx",
		Label:   "CII — Factur-X",
		Group:   "CII",
		MIME:    "application/xml",
		convert: convertCII(cii.ContextFacturXV1),
	},
	{
		ID:      "cii-zugferd",
		Label:   "CII — ZUGFeRD",
		Group:   "CII",
		MIME:    "application/xml",
		convert: convertCII(cii.ContextZUGFeRDV2),
	},
	{
		ID:      "cii-xrechnung",
		Label:   "CII — XRechnung",
		Group:   "CII",
		MIME:    "application/xml",
		convert: convertCII(cii.ContextXRechnungV3),
	},
	{
		ID:    "fatturapa",
		Label: "FatturaPA",
		Group: "Italy",
		MIME:  "application/xml",
		convert: func(_ context.Context, env *gobl.Envelope) ([]byte, error) {
			doc, err := fatturapa.Convert(env)
			if err != nil {
				return nil, err
			}
			return doc.Bytes()
		},
	},
	{
		ID:    "html",
		Label: "HTML preview",
		Group: "Preview",
		MIME:  "text/html; charset=utf-8",
		convert: func(ctx context.Context, env *gobl.Envelope) ([]byte, error) {
			return goblhtml.Render(ctx, env)
		},
	},
}

// convertUBL builds a UBL converter closure bound to a specific context.
// gobl.ubl.Convert handles addon injection internally via ensureAddons.
func convertUBL(cx ubl.Context) func(context.Context, *gobl.Envelope) ([]byte, error) {
	return func(_ context.Context, env *gobl.Envelope) ([]byte, error) {
		doc, err := ubl.Convert(env, ubl.WithContext(cx))
		if err != nil {
			return nil, err
		}
		return ubl.Bytes(doc)
	}
}

// convertCII builds a CII converter closure that auto-injects any missing
// addons the chosen context requires, recalculates totals, and then serialises
// to XML bytes.
func convertCII(cx cii.Context) func(context.Context, *gobl.Envelope) ([]byte, error) {
	return func(_ context.Context, env *gobl.Envelope) ([]byte, error) {
		if err := ensureInvoiceAddons(env, cx.Addons); err != nil {
			return nil, err
		}
		raw, err := cii.Convert(env, cii.WithContext(cx))
		if err != nil {
			return nil, err
		}
		doc, ok := raw.(*cii.Invoice)
		if !ok {
			return nil, fmt.Errorf("cii: unexpected document type %T", raw)
		}
		return doc.Bytes()
	}
}

// ensureInvoiceAddons appends any missing required addons to the envelope's
// bill.Invoice and recalculates so that scenario-driven extensions are
// populated before conversion.
func ensureInvoiceAddons(env *gobl.Envelope, required []cbc.Key) error {
	if len(required) == 0 {
		return nil
	}
	inv, ok := env.Extract().(*bill.Invoice)
	if !ok {
		return nil
	}
	existing := inv.GetAddons()
	missing := make([]cbc.Key, 0, len(required))
	for _, a := range required {
		if !a.In(existing...) {
			missing = append(missing, a)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	inv.SetAddons(append(existing, missing...)...)
	if err := inv.Calculate(); err != nil {
		return fmt.Errorf("calculate with addons %v: %w", missing, err)
	}
	return nil
}

// FormatInfo is the public subset of a Format, suitable for rendering or
// serialising to the browser.
type FormatInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	MIME  string `json:"mime"`
}

// formatList returns the public format metadata in registration order.
func formatList() []FormatInfo {
	out := make([]FormatInfo, 0, len(formats))
	for _, f := range formats {
		out = append(out, FormatInfo{f.ID, f.Label, f.Group, f.MIME})
	}
	return out
}

// findFormat looks up a format by ID.
func findFormat(id string) (Format, bool) {
	for _, f := range formats {
		if f.ID == id {
			return f, true
		}
	}
	return Format{}, false
}

// handleFormats returns the list of available output formats.
func handleFormats(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(formatList())
}

// handleConvert accepts a GOBL envelope in the body and returns the converted
// output for the requested format. Errors are returned as a JSON payload in
// the same shape as the core /v0/build endpoint — including structured
// rules.Fault entries when the converter fails validation — so the editor's
// error panel can render them identically.
func handleConvert(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("format")
	f, ok := findFormat(id)
	if !ok {
		goblapi.WriteError(w, gobl.ErrInput.WithReason("unknown format: %s", id))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		goblapi.WriteError(w, gobl.ErrInput.WithCause(fmt.Errorf("read body: %w", err)))
		return
	}

	env, err := parseConvertBody(body)
	if err != nil {
		goblapi.WriteError(w, asGoblError(err))
		return
	}

	out, err := safeConvert(r.Context(), f, env)
	if err != nil {
		goblapi.WriteError(w, asGoblError(err))
		return
	}

	w.Header().Set("Content-Type", f.MIME)
	_, _ = w.Write(out)
}

// parseConvertBody accepts either a full GOBL envelope or a bare document
// (e.g. a bill.Invoice) — the editor sends the latter when "Envelop" is
// unchecked — and returns a calculated envelope ready for conversion.
func parseConvertBody(body []byte) (*gobl.Envelope, error) {
	// Try the envelope shape first — it's the output of a built /v0/build
	// with envelop=true.
	env := new(gobl.Envelope)
	if err := json.Unmarshal(body, env); err == nil && env.Document != nil && env.Extract() != nil {
		return env, nil
	}
	// Otherwise parse via the schema registry and wrap.
	doc, err := gobl.Parse(body)
	if err != nil {
		return nil, gobl.ErrInput.WithCause(fmt.Errorf("parse document: %w", err))
	}
	wrapped, err := gobl.Envelop(doc)
	if err != nil {
		return nil, err
	}
	return wrapped, nil
}

// asGoblError normalises a converter error into a *gobl.Error so it serialises
// with the {key, faults, message} shape that the editor's error panel expects.
// Errors that already are *gobl.Error pass through untouched, preserving their
// rules.Faults cause for path-level rendering.
func asGoblError(err error) *gobl.Error {
	var ge *gobl.Error
	if errors.As(err, &ge) {
		return ge
	}
	return gobl.ErrInternal.WithCause(err)
}

// safeConvert wraps the converter call with panic recovery so upstream bugs
// (e.g. missing regime branches in gobl.html) surface as 4xx errors instead of
// crashing the server.
func safeConvert(ctx context.Context, f Format, env *gobl.Envelope) (out []byte, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("converter panicked: %v", rec)
		}
	}()
	return f.convert(ctx, env)
}
