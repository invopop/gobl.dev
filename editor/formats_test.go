package editor

import (
	"testing"

	_ "github.com/invopop/gobl.dev/bundle"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportFormats(t *testing.T) {
	keys := make(map[string]exportFormat)
	for _, f := range exportFormats() {
		keys[f.Key] = f
	}
	require.Contains(t, keys, "ubl+en16931-v2017")
	assert.Equal(t, "UBL EN 16931:2017", keys["ubl+en16931-v2017"].Label)
	assert.Equal(t, []cbc.Key{"eu-en16931-v2017"}, keys["ubl+en16931-v2017"].Addons)
	assert.Contains(t, keys, "cii+en16931-v2017")
	assert.NotContains(t, keys, "ubl", "import-only formats are not offered")
}

func TestAddonRequirements(t *testing.T) {
	reqs := addonRequirements()
	assert.Contains(t, reqs[cbc.Key("de-xrechnung-v3")], cbc.Key("eu-en16931-v2017"))
	assert.NotContains(t, reqs, cbc.Key("eu-en16931-v2017"))
}

func TestFormatsFor(t *testing.T) {
	invoice := schema.Lookup(bill.Invoice{})
	keys := func(list []exportFormat) []string {
		out := make([]string, len(list))
		for i, f := range list {
			out[i] = f.Key
		}
		return out
	}
	assert.Contains(t, keys(formatsFor(invoice, []cbc.Key{"eu-en16931-v2017"})), "ubl+en16931-v2017")
	assert.Contains(t, keys(formatsFor(invoice, []cbc.Key{"de-xrechnung-v3"})), "cii+en16931-v2017", "required addons are expanded")
	assert.NotContains(t, keys(formatsFor(invoice, nil)), "ubl+en16931-v2017")
	assert.Empty(t, formatsFor("https://gobl.org/draft-0/note/message", []cbc.Key{"eu-en16931-v2017"}))
}
