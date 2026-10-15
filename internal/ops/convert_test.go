package ops

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	_ "github.com/invopop/gobl.dev/bundle"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormats(t *testing.T) {
	keys := make([]cbc.Key, 0)
	for _, f := range Formats() {
		keys = append(keys, f.Key)
	}
	assert.Contains(t, keys, cbc.Key("ubl+en16931-v2017"))
	assert.Contains(t, keys, cbc.Key("cii+en16931-v2017"))
}

func TestExport(t *testing.T) {
	ctx := context.Background()

	t.Run("document to UBL", func(t *testing.T) {
		out, err := Export(ctx, &ExportOptions{
			ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/convert-invoice.json")},
			Formats:      []cbc.Key{"ubl+en16931-v2017"},
		})
		require.NoError(t, err)
		assert.Equal(t, cbc.Key("ubl+en16931-v2017"), out.Format.Key)
		assert.Contains(t, string(out.Data), "<Invoice")
		assert.Contains(t, string(out.Data), "urn:cen.eu:en16931:2017")
	})

	t.Run("first accepted format is used", func(t *testing.T) {
		out, err := Export(ctx, &ExportOptions{
			ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/convert-invoice.json")},
			Formats:      []cbc.Key{"cii+en16931-v2017", "ubl+en16931-v2017"},
		})
		require.NoError(t, err)
		assert.Equal(t, cbc.Key("cii+en16931-v2017"), out.Format.Key)
		assert.Contains(t, string(out.Data), "CrossIndustryInvoice")
	})

	t.Run("no formats", func(t *testing.T) {
		_, err := Export(ctx, &ExportOptions{
			ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/convert-invoice.json")},
		})
		assert.ErrorIs(t, err, gobl.ErrInput)
	})

	t.Run("unknown format", func(t *testing.T) {
		_, err := Export(ctx, &ExportOptions{
			ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/convert-invoice.json")},
			Formats:      []cbc.Key{"foo+bar"},
		})
		assert.ErrorIs(t, err, convert.ErrUnknownFormat)
	})

	t.Run("document missing required addon", func(t *testing.T) {
		_, err := Export(ctx, &ExportOptions{
			ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/success.json")},
			Formats:      []cbc.Key{"ubl+en16931-v2017"},
		})
		assert.ErrorIs(t, err, convert.ErrNotSupported)
	})
}

func TestImport(t *testing.T) {
	ctx := context.Background()

	for _, key := range []cbc.Key{"ubl+en16931-v2017", "cii+en16931-v2017"} {
		t.Run("round trip "+key.String(), func(t *testing.T) {
			out, err := Export(ctx, &ExportOptions{
				ParseOptions: &ParseOptions{Input: testFileReader(t, "testdata/convert-invoice.json")},
				Formats:      []cbc.Key{key},
			})
			require.NoError(t, err)

			res, err := Import(ctx, &ImportOptions{Input: bytes.NewReader(out.Data)})
			require.NoError(t, err)
			assert.Equal(t, key, res.Format.Key, "detected format")
			inv, ok := res.Envelope.Extract().(*bill.Invoice)
			require.True(t, ok)
			// UBL and CII carry the series and code as a single identifier.
			assert.Equal(t, cbc.Code("SAMPLE-001"), inv.Code)
			assert.Equal(t, "Provide One GmbH", inv.Supplier.Name)

			// Limiting detection to another format fails.
			other := cbc.Key("cii+en16931-v2017")
			if key == other {
				other = "ubl+en16931-v2017"
			}
			_, err = Import(ctx, &ImportOptions{
				Input:   bytes.NewReader(out.Data),
				Formats: []cbc.Key{other},
			})
			assert.ErrorIs(t, err, convert.ErrUnknownFormat)
		})
	}

	t.Run("empty input", func(t *testing.T) {
		_, err := Import(ctx, &ImportOptions{Input: strings.NewReader("")})
		assert.ErrorIs(t, err, gobl.ErrInput)
	})

	t.Run("unrecognized input", func(t *testing.T) {
		_, err := Import(ctx, &ImportOptions{Input: strings.NewReader(`{"foo":"bar"}`)})
		assert.ErrorIs(t, err, convert.ErrUnknownFormat)
	})
}
