package editor

import (
	"embed"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/tax"
)

//go:embed examples/*.json
var examplesFS embed.FS

// defaultExample is the ID of the example loaded when the editor opens.
const defaultExample = "us"

// example is a bare-minimum GOBL document for a region, offered in the
// editor's example picker. The region and label are derived from the
// document itself, so adding an example only requires adding its file.
type example struct {
	ID     string          `json:"id"`
	Region string          `json:"region"`
	Label  string          `json:"label"`
	Addons []cbc.Key       `json:"addons,omitempty"`
	Doc    json.RawMessage `json:"doc"`
}

// examples are loaded on first use, as the names of the addons they use are
// only available once the bundle has registered them.
var examples = sync.OnceValue(mustLoadExamples)

func mustLoadExamples() []*example {
	entries, err := examplesFS.ReadDir("examples")
	if err != nil {
		panic(err)
	}
	list := make([]*example, 0, len(entries))
	for _, e := range entries {
		data, err := examplesFS.ReadFile(path.Join("examples", e.Name()))
		if err != nil {
			panic(err)
		}
		ex, err := newExample(strings.TrimSuffix(e.Name(), ".json"), data)
		if err != nil {
			panic(e.Name() + ": " + err.Error())
		}
		list = append(list, ex)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Region != list[j].Region {
			return list[i].Region < list[j].Region
		}
		// Examples without addons come first in their region.
		if len(list[i].Addons) != len(list[j].Addons) {
			return len(list[i].Addons) < len(list[j].Addons)
		}
		return list[i].Label < list[j].Label
	})
	return list
}

func newExample(id string, data []byte) (*example, error) {
	var doc struct {
		Regime   l10n.TaxCountryCode `json:"$regime"`
		Addons   []cbc.Key           `json:"$addons"`
		Supplier struct {
			TaxID struct {
				Country l10n.TaxCountryCode `json:"country"`
			} `json:"tax_id"`
		} `json:"supplier"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	country := doc.Regime
	if country == "" {
		country = doc.Supplier.TaxID.Country
	}
	region := country.String()
	if r := tax.RegimeDefFor(country.Code()); r != nil {
		region = r.Name.In(i18n.EN)
	}

	label := "Standard"
	if len(doc.Addons) > 0 {
		names := make([]string, len(doc.Addons))
		for i, k := range doc.Addons {
			names[i] = k.String()
			if a := tax.AddonForKey(k); a != nil {
				names[i] = a.Name.In(i18n.EN)
			}
		}
		label = strings.Join(names, ", ")
	}

	return &example{
		ID:     id,
		Region: region,
		Label:  label,
		Addons: doc.Addons,
		Doc:    json.RawMessage(data),
	}, nil
}
