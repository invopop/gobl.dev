package editor

import (
	"bytes"
	"context"
	"slices"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/schema"
)

// formatLabels names the formats registered with GOBL's convert package
// that documents can be exported into, by key. Formats that cannot be
// imported, as their documents cannot be told apart from another format's,
// are marked as such.
func formatLabels() map[string]string {
	labels := make(map[string]string)
	for _, f := range convert.Formats() {
		if len(f.Export) == 0 {
			continue
		}
		label := f.Name.In(i18n.EN)
		if label == "" {
			label = f.Key.String()
		}
		if len(f.Import) == 0 {
			label += " (export only)"
		}
		labels[f.Key.String()] = label
	}
	return labels
}

// importable reports whether the document, exported into the format with
// the key, is detected as that format when imported back into GOBL.
func importable(key string, schemaID schema.ID, doc []byte) bool {
	f := convert.FormatFor(cbc.Key(key))
	if f == nil || !slices.Contains(f.Import, schemaID) {
		return false
	}
	out, err := ops.Export(context.Background(), &ops.ExportOptions{
		ParseOptions: &ops.ParseOptions{Input: bytes.NewReader(doc)},
		Formats:      []cbc.Key{f.Key},
	})
	if err != nil {
		return false
	}
	got, err := convert.Detect(out.Data)
	return err == nil && got.Key == f.Key
}

// acceptedFormats lists the keys of the formats that accept the document for
// export, in registration order. Only the formats' converters can tell, so
// the document is checked against each of them.
func acceptedFormats(doc []byte) ([]string, error) {
	checks, err := ops.CheckFormats(context.Background(), &ops.ParseOptions{
		Input: bytes.NewReader(doc),
	})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for _, c := range checks {
		if c.Accepted {
			keys = append(keys, c.Key.String())
		}
	}
	return keys, nil
}

// bootstrap is the initial state rendered into the page as JSON and read by
// the editor's Alpine component.
func bootstrap() map[string]any {
	return map[string]any{
		"examples":       examples(),
		"defaultExample": defaultExample,
		"formats":        formatLabels(),
	}
}
