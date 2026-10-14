package editor

import (
	"slices"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/schema"
	"github.com/invopop/gobl/tax"
)

// exportFormat describes a format registered with GOBL's convert package
// that documents can be exported into. The editor offers a format only when
// the document's schema is listed in Export and the document includes every
// addon in Addons.
type exportFormat struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Addons []cbc.Key   `json:"addons,omitempty"`
	Export []schema.ID `json:"export"`
}

// exportFormats lists the registered formats that documents can be exported
// into, in registration order.
func exportFormats() []exportFormat {
	list := make([]exportFormat, 0)
	for _, f := range convert.Formats() {
		if len(f.Export) == 0 {
			continue
		}
		label := f.Name.In(i18n.EN)
		if label == "" {
			label = f.Key.String()
		}
		list = append(list, exportFormat{
			Key:    f.Key.String(),
			Label:  label,
			Addons: f.Addons,
			Export: f.Export,
		})
	}
	return list
}

// formatsFor lists the export formats for documents of the schema that
// declare the addons. It mirrors the editor's client-side matching: addons
// are expanded with those they require, as GOBL does when building, and
// every addon a format lists must be present.
func formatsFor(schemaID schema.ID, addons []cbc.Key) []exportFormat {
	all := expandAddons(addons)
	list := make([]exportFormat, 0)
	for _, f := range exportFormats() {
		if !slices.Contains(f.Export, schemaID) {
			continue
		}
		if !containsAll(all, f.Addons) {
			continue
		}
		list = append(list, f)
	}
	return list
}

// importable reports whether the example can be offered in import mode, as
// the output of one of its export formats.
func (ex *example) importable() bool {
	return len(formatsFor(schema.Lookup(bill.Invoice{}), ex.Addons)) > 0
}

func expandAddons(keys []cbc.Key) []cbc.Key {
	reqs := addonRequirements()
	out := slices.Clone(keys)
	for i := 0; i < len(out); i++ {
		for _, r := range reqs[out[i]] {
			if !slices.Contains(out, r) {
				out = append(out, r)
			}
		}
	}
	return out
}

func containsAll(list, keys []cbc.Key) bool {
	for _, k := range keys {
		if !slices.Contains(list, k) {
			return false
		}
	}
	return true
}

// addonRequirements maps each registered addon that requires others to the
// keys it requires, so the editor can work out the addons a document will
// include once built.
func addonRequirements() map[cbc.Key][]cbc.Key {
	reqs := make(map[cbc.Key][]cbc.Key)
	for _, a := range tax.AllAddonDefs() {
		if len(a.Requires) > 0 {
			reqs[a.Key] = a.Requires
		}
	}
	return reqs
}

// bootstrap is the initial state rendered into the page as JSON and read by
// the editor's Alpine component.
func bootstrap() map[string]any {
	return map[string]any{
		"examples":       examples(),
		"defaultExample": defaultExample,
		"formats":        exportFormats(),
		"requires":       addonRequirements(),
	}
}
