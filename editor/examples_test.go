package editor

import (
	"bytes"
	"context"
	"testing"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
	"github.com/invopop/gobl/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExamples(t *testing.T) {
	list := examples()
	require.NotEmpty(t, list)
	ids := make([]string, len(list))
	for i, ex := range list {
		ids[i] = ex.ID
	}
	assert.Contains(t, ids, defaultExample)

	for _, ex := range list {
		t.Run(ex.ID, func(t *testing.T) {
			assert.NotEmpty(t, ex.Region)
			assert.NotEmpty(t, ex.Label)
			assert.Greater(t, len(ex.Region), 2, "region name should come from the regime")

			_, err := ops.Build(context.Background(), &ops.BuildOptions{
				ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(ex.Doc)},
			})
			require.NoError(t, err)

			// Every format that accepts the example converts it.
			for _, key := range ex.Formats {
				_, err := ops.Export(context.Background(), &ops.ExportOptions{
					ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(ex.Doc)},
					Formats:      []cbc.Key{cbc.Key(key)},
				})
				assert.NoError(t, err, "export to %s", key)
			}
		})
	}
}

// TestExamplesCoverFormats ensures every format that exports invoices is
// accepted by at least one example, so all of them can be tried out in the
// editor.
func TestExamplesCoverFormats(t *testing.T) {
	covered := make(map[string]bool)
	for _, ex := range examples() {
		for _, k := range ex.Formats {
			covered[k] = true
		}
	}
	invoice := schema.Lookup(bill.Invoice{})
	for _, f := range convert.Formats() {
		for _, s := range f.Export {
			if s == invoice {
				assert.True(t, covered[f.Key.String()], "no example is accepted by %s", f.Key)
			}
		}
	}
}

func TestExampleFormatPreference(t *testing.T) {
	preferred := map[string]string{
		"de-xrechnung": "cii+de-xrechnung-v3",
		"fr-facturx":   "cii+fr-facturx-v1",
		"sa-zatca":     "ubl+sa-zatca-v1",
		"de-en16931":   "cii+en16931-v2017",
	}
	for _, ex := range examples() {
		if want, ok := preferred[ex.ID]; ok {
			require.NotEmpty(t, ex.Formats, ex.ID)
			assert.Equal(t, want, ex.Formats[0], ex.ID)
		}
	}
}

// TestExampleSamples ensures every sample offered for import is detected as
// the format it was exported into, so importing it is faithful.
func TestExampleSamples(t *testing.T) {
	for _, ex := range examples() {
		if ex.Sample == "" {
			continue
		}
		t.Run(ex.ID, func(t *testing.T) {
			out, err := ops.Export(context.Background(), &ops.ExportOptions{
				ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(ex.Doc)},
				Formats:      []cbc.Key{cbc.Key(ex.Sample)},
			})
			require.NoError(t, err)
			res, err := ops.Import(context.Background(), &ops.ImportOptions{Input: bytes.NewReader(out.Data)})
			require.NoError(t, err)
			assert.Equal(t, ex.Sample, res.Format.Key.String())
		})
	}
	samples := map[string]string{
		"de-zugferd":  "cii+de-zugferd-v2+basic",
		"es-facturae": "es-facturae-v3",
		"fr-facturx":  "cii+fr-facturx-v1+basic",
		"sa-zatca":    "ubl+sa-zatca-v1",
	}
	for _, ex := range examples() {
		if want, ok := samples[ex.ID]; ok {
			assert.Equal(t, want, ex.Sample, ex.ID)
		}
	}
}

func TestExamplesOrder(t *testing.T) {
	list := examples()
	for i := 1; i < len(list); i++ {
		assert.LessOrEqual(t, list[i-1].Region, list[i].Region)
	}
}
