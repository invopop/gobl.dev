package ops_test

import (
	"encoding/json"
	"testing"

	_ "github.com/invopop/gobl"
	"github.com/invopop/gobl.dev/internal/ops"

	// Register the full GOBL addon set so externally-implemented addons are
	// covered, not just those compiled into core GOBL.
	_ "github.com/invopop/gobl.dev/bundle"

	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddonData(t *testing.T) {
	t.Run("core addon", func(t *testing.T) {
		d, err := ops.AddonData("es-verifactu-v1")
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(d, &got))
		assert.Equal(t, "es-verifactu-v1", got["key"])
		assert.Equal(t, "https://gobl.org/draft-0/tax/addon-def", got["$schema"])
	})

	// Addons living in their own modules ship no file in core GOBL's embedded
	// data directory, so they have to be rendered from the registry.
	t.Run("externally implemented addon", func(t *testing.T) {
		d, err := ops.AddonData("mx-cfdi-v4")
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(d, &got))
		assert.Equal(t, "mx-cfdi-v4", got["key"])
		assert.NotEmpty(t, got["extensions"], "expected the full definition, not just a stub")
	})

	t.Run("json suffix accepted", func(t *testing.T) {
		with, err := ops.AddonData("mx-cfdi-v4.json")
		require.NoError(t, err)
		without, err := ops.AddonData("mx-cfdi-v4")
		require.NoError(t, err)
		assert.Equal(t, without, with)
	})

	t.Run("unknown key", func(t *testing.T) {
		_, err := ops.AddonData("nonexistent-addon")
		assert.ErrorIs(t, err, ops.ErrAddonNotFound)
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := ops.AddonData("")
		assert.ErrorIs(t, err, ops.ErrAddonNotFound)
	})

	// Guards the invariant that broke: every registered addon must render,
	// whichever module implements it.
	t.Run("every registered addon renders", func(t *testing.T) {
		defs := tax.AllAddonDefs()
		require.NotEmpty(t, defs)

		for _, def := range defs {
			key := def.Key.String()
			t.Run(key, func(t *testing.T) {
				d, err := ops.AddonData(key)
				require.NoError(t, err)

				var got map[string]any
				require.NoError(t, json.Unmarshal(d, &got))
				assert.Equal(t, key, got["key"])
			})
		}
	})
}

func TestAddonKeys(t *testing.T) {
	keys := ops.AddonKeys()
	require.NotEmpty(t, keys)

	assert.Len(t, keys, len(tax.AllAddonDefs()))
	assert.Contains(t, keys, "es-verifactu-v1", "core addon missing")
	assert.Contains(t, keys, "mx-cfdi-v4", "externally-implemented addon missing")
}
