package bundle

import (
	"testing"

	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApprovedAddonsAreBundled guards against drift between core's approved
// external-addon list and what this bundle actually registers. Every key in
// tax.ApprovedAddons must resolve to a registered addon once the bundle is
// loaded (this test runs in package bundle, so its blank imports are in effect).
//
// A failure means either a blank import is missing from bundle.go, or — if an
// addon module pins a core whose major version differs from the one built here
// — the addon registered into a different tax registry and is invisible to this
// binary.
func TestApprovedAddonsAreBundled(t *testing.T) {
	approved := tax.ApprovedAddons()
	require.NotEmpty(t, approved, "core declares no approved external addons")

	for _, ea := range approved {
		assert.NotNilf(t, tax.AddonForKey(ea.Key),
			"approved addon %q (%s) is not registered — add a blank import to bundle.go",
			ea.Key, ea.Module)
	}
}
