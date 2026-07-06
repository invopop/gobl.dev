package bundle_test

import (
	"testing"

	_ "github.com/invopop/gobl"
	"github.com/invopop/gobl/tax"

	_ "github.com/invopop/gobl.dev/bundle"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownUnavailable exempts approved addon keys the bundle deliberately does
// not provide, mapped to the reason. Keep this empty whenever possible.
var knownUnavailable = map[string]string{}

// TestApprovedAddonsAvailable ensures every addon approved by GOBL is
// registered via the bundle's imports.
func TestApprovedAddonsAvailable(t *testing.T) {
	approved := tax.ApprovedAddons()
	require.NotEmpty(t, approved, "expected gobl to expose approved addons; is gobl imported?")

	for _, ea := range approved {
		t.Run(ea.Key.String(), func(t *testing.T) {
			if reason, ok := knownUnavailable[ea.Key.String()]; ok {
				t.Skipf("known gap for %q: %s", ea.Key, reason)
			}
			assert.NotNilf(t, tax.AddonForKey(ea.Key),
				"approved addon %q (module %s) is not registered; add a blank import for %s/addon to bundle.go",
				ea.Key, ea.Module, ea.Module)
		})
	}
}
