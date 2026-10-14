package editor

import (
	"github.com/invopop/gobl/convert"
	"github.com/invopop/gobl/i18n"
)

// exportFormat is an entry in the editor's export picker.
type exportFormat struct {
	Key   string
	Label string
}

// exportFormats lists the formats registered with GOBL's convert package that
// documents can be exported into, in registration order.
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
		list = append(list, exportFormat{Key: f.Key.String(), Label: label})
	}
	return list
}
