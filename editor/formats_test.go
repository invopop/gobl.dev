package editor

import (
	"testing"

	_ "github.com/invopop/gobl.cii"
	_ "github.com/invopop/gobl.ubl"
	"github.com/stretchr/testify/assert"
)

func TestExportFormats(t *testing.T) {
	list := exportFormats()
	assert.Contains(t, list, exportFormat{Key: "ubl+en16931-v2017", Label: "UBL EN 16931:2017"})
	assert.Contains(t, list, exportFormat{Key: "cii+en16931-v2017", Label: "CII EN 16931:2017"})
}
