package bundle_test

import (
	"testing"

	// Importing gobl registers the in-core addons and the curated list of
	// approved external addons (see gobl's addons/external.go).
	_ "github.com/invopop/gobl"
	"github.com/invopop/gobl/tax"

	// Importing the bundle blank-registers every external addon module that
	// GOBL.dev ships with (see bundle.go).
	_ "github.com/invopop/gobl.dev/bundle"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownUnavailable lists approved addon keys that the bundle deliberately does
// not (or cannot) provide, mapped to the reason. Entries here are exempted from
// the strict availability check below. Keep this map empty whenever possible —
// every entry is a gap in GOBL.dev's addon coverage.
//
// Remove an entry (and rely on the strict check) as soon as the underlying
// cause is resolved.
var knownUnavailable = map[string]string{}

// TestApprovedAddonsAvailable ensures that every addon GOBL knows about is
// actually available (registered) through the bundle. GOBL's approved list
// (tax.ApprovedAddons) only records that a key is endorsed; the addon becomes
// usable at runtime only once its module is imported. The bundle is the single
// place where GOBL.dev imports those modules, so if GOBL approves a new addon
// that the bundle doesn't import, this test fails and points at the missing
// module.
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
