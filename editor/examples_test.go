package editor

import (
	"bytes"
	"context"
	"testing"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownFormatGaps lists formats whose register metadata does not capture
// every addon they need, so the editor may offer them for documents they then
// reject, mapped to the reason. Keep this empty whenever possible.
var knownFormatGaps = map[string]string{
	"ubl+peppol+bis-v3": "also exports bill/status, so the invoice-only EN 16931 addon is checked in Accepts rather than listed in Addons",
}

func TestExamples(t *testing.T) {
	list := examples()
	require.NotEmpty(t, list)
	ids := make([]string, len(list))
	for i, ex := range list {
		ids[i] = ex.ID
	}
	assert.Contains(t, ids, defaultExample)

	invoice := schema.Lookup(bill.Invoice{})
	for _, ex := range list {
		t.Run(ex.ID, func(t *testing.T) {
			assert.NotEmpty(t, ex.Region)
			assert.NotEmpty(t, ex.Label)
			assert.NotEqual(t, ex.Region, ex.Region[:min(2, len(ex.Region))], "region name should come from the regime")

			_, err := ops.Build(context.Background(), &ops.BuildOptions{
				ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(ex.Doc)},
			})
			require.NoError(t, err)

			// Every format the editor offers the example must accept it.
			for _, f := range formatsFor(invoice, ex.Addons) {
				if _, ok := knownFormatGaps[f.Key]; ok {
					continue
				}
				_, err := ops.Export(context.Background(), &ops.ExportOptions{
					ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(ex.Doc)},
					Formats:      []cbc.Key{cbc.Key(f.Key)},
				})
				assert.NoError(t, err, "export to %s", f.Key)
			}
		})
	}
}

func TestExamplesOrder(t *testing.T) {
	list := examples()
	for i := 1; i < len(list); i++ {
		assert.LessOrEqual(t, list[i-1].Region, list[i].Region)
	}
}
