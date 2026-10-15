package editor

import (
	"bytes"
	"context"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/convert"
	"github.com/invopop/gobl/i18n"
)

// formatLabels names the formats registered with GOBL's convert package
// that documents can be exported into, by key.
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
		labels[f.Key.String()] = label
	}
	return labels
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
