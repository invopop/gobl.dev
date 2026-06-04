// Package examples holds curated starter invoices exposed through the editor.
package examples

import (
	"embed"
	"fmt"
)

// Example describes a curated starter document.
type Example struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Country     string `json:"country"`
	Addon       string `json:"addon,omitempty"`
	Description string `json:"description,omitempty"`
}

// Group is a set of examples that share the same document Type. Returned by
// Grouped in the order the types first appear in the catalog.
type Group struct {
	Type  string    `json:"type"`
	Items []Example `json:"items"`
}

//go:embed *.json
var files embed.FS

// Document types. More may appear alongside Invoice as the catalog grows
// (e.g. "Credit Note", "Delivery", "Payment").
const (
	TypeInvoice = "Invoice"
)

// catalog is the ordered list of available examples. Ordering: plain country
// variant first, then addon variants, grouped by country alpha-code.
var catalog = []Example{
	{ID: "de", Label: "DE — Germany", Type: TypeInvoice, Country: "DE", Description: "Standard German VAT invoice."},
	{ID: "de-xrechnung", Label: "DE — XRechnung", Type: TypeInvoice, Country: "DE", Addon: "de-xrechnung-v3", Description: "German public-sector e-invoice."},
	{ID: "es", Label: "ES — Spain", Type: TypeInvoice, Country: "ES", Description: "Standard Spanish VAT invoice."},
	{ID: "es-verifactu", Label: "ES — VERI*FACTU", Type: TypeInvoice, Country: "ES", Addon: "es-verifactu-v1", Description: "Spanish real-time VAT reporting (AEAT)."},
	{ID: "es-tbai", Label: "ES — TicketBAI", Type: TypeInvoice, Country: "ES", Addon: "es-tbai-v1", Description: "Basque Country e-invoicing."},
	{ID: "fr", Label: "FR — France", Type: TypeInvoice, Country: "FR", Description: "Standard French VAT invoice."},
	{ID: "fr-facturx", Label: "FR — Factur-X", Type: TypeInvoice, Country: "FR", Addon: "fr-facturx-v1", Description: "French hybrid PDF/XML e-invoice."},
	{ID: "gb", Label: "GB — United Kingdom", Type: TypeInvoice, Country: "GB", Description: "UK VAT invoice."},
	{ID: "it-fatturapa", Label: "IT — FatturaPA", Type: TypeInvoice, Country: "IT", Addon: "it-sdi-v1", Description: "Italian SdI electronic invoice."},
	{ID: "nl", Label: "NL — Netherlands", Type: TypeInvoice, Country: "NL", Description: "Dutch VAT invoice."},
	{ID: "pt", Label: "PT — Portugal", Type: TypeInvoice, Country: "PT", Description: "Portuguese VAT invoice."},
	{ID: "us", Label: "US — United States", Type: TypeInvoice, Country: "US", Description: "Basic sales-tax invoice."},
}

// All returns the curated examples in display order.
func All() []Example {
	return catalog
}

// Grouped returns the examples bucketed by Type, in the order each type
// first appears in the catalog. Items within each group preserve their
// catalog ordering.
func Grouped() []Group {
	groups := make([]Group, 0)
	index := map[string]int{}
	for _, e := range catalog {
		i, ok := index[e.Type]
		if !ok {
			index[e.Type] = len(groups)
			groups = append(groups, Group{Type: e.Type, Items: []Example{e}})
			continue
		}
		groups[i].Items = append(groups[i].Items, e)
	}
	return groups
}

// Get returns the raw JSON for a given example ID.
func Get(id string) ([]byte, bool) {
	for _, e := range catalog {
		if e.ID == id {
			data, err := files.ReadFile(e.ID + ".json")
			if err != nil {
				return nil, false
			}
			return data, true
		}
	}
	return nil, false
}

// Find returns the example metadata for a given ID.
func Find(id string) (Example, bool) {
	for _, e := range catalog {
		if e.ID == id {
			return e, true
		}
	}
	return Example{}, false
}

// DefaultFor returns the preferred example for the given ISO-3166-1 alpha-2
// country code (upper-case). It picks the first entry whose Country matches,
// falling back to the US example if no match is found. The plain-country
// variant (no addon) wins when both a plain and addon variant exist, because
// of the ordering inside catalog.
func DefaultFor(country string) Example {
	for _, e := range catalog {
		if e.Country == country {
			return e
		}
	}
	for _, e := range catalog {
		if e.ID == "us" {
			return e
		}
	}
	panic(fmt.Errorf("examples: us fallback not found"))
}
