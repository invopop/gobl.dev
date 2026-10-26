package editor

import (
	"testing"

	_ "github.com/invopop/gobl.dev/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatLabels(t *testing.T) {
	labels := formatLabels()
	assert.Equal(t, "UBL EN 16931:2017", labels["ubl+en16931-v2017"])
	assert.Equal(t, "CII XRechnung 3", labels["cii+de-xrechnung-v3"])
	assert.NotContains(t, labels, "ubl", "import-only formats are not offered")
	assert.Equal(t, "CII France CTC Factur-X CIUS (export only)", labels["cii+fr-ctc-facturx-cius-v1"])
}

func TestAcceptedFormats(t *testing.T) {
	keys, err := acceptedFormats([]byte(`{
		"$schema": "https://gobl.org/draft-0/bill/invoice",
		"currency": "USD",
		"supplier": {"name": "Acme Inc.", "tax_id": {"country": "US"}},
		"customer": {"name": "Sample Customer"},
		"lines": [{"quantity": "1", "item": {"name": "Test", "price": "10.00"}}]
	}`))
	require.NoError(t, err)
	assert.Empty(t, keys, "formats need addons the document lacks")

	_, err = acceptedFormats([]byte(`{"$schema": "https://gobl.org/draft-0/bill/invoice"}`))
	assert.Error(t, err, "the document must build")
}
