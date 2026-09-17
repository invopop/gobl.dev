package ops

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/schema"
	"github.com/invopop/gobl/tax"
)

// ErrAddonNotFound is returned by [AddonData] when no addon is registered
// under the requested key.
var ErrAddonNotFound = errors.New("addon not found")

// AddonData renders the full definition of the addon registered under key as
// indented JSON, wrapped in a schema object.
//
// The definition comes from the in-memory registry rather than GOBL's embedded
// data directory. Addons implemented in their own modules (gobl.mx.cfdi,
// gobl.it.sdi, and so on) register themselves via init() when blank-imported by
// the bundle, but ship no file inside core GOBL's data/addons — reading from
// there would serve only the addons still compiled into core. The output
// matches core GOBL's generated files byte for byte, since this mirrors what
// its addons/generate.go does.
//
// A ".json" suffix on key is accepted and ignored.
func AddonData(key string) ([]byte, error) {
	key = strings.TrimSuffix(key, ".json")

	def := tax.AddonForKey(cbc.Key(key))
	if def == nil {
		return nil, ErrAddonNotFound
	}

	obj, err := schema.NewObject(def)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(obj, "", "  ")
}

// AddonKeys returns the key of every registered addon, ordered as the registry
// reports them.
func AddonKeys() []string {
	defs := tax.AllAddonDefs()
	keys := make([]string, len(defs))
	for i, def := range defs {
		keys[i] = string(def.Key)
	}
	return keys
}
