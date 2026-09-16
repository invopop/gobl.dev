package api

import (
	"errors"
	"net/http"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/tax"
)

type addonSummary struct {
	Key         string      `json:"key"`
	Name        i18n.String `json:"name"`
	Description i18n.String `json:"description,omitempty"`
	Requires    []cbc.Key   `json:"requires,omitempty"`
}

func handleAddonList(w http.ResponseWriter, _ *http.Request) {
	defs := tax.AllAddonDefs()
	items := make([]addonSummary, len(defs))
	for i, a := range defs {
		items[i] = addonSummary{
			Key:         string(a.Key),
			Name:        a.Name,
			Description: a.Description,
			Requires:    a.Requires,
		}
	}
	WriteJSON(w, map[string]any{"addons": items})
}

func handleAddon(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		WriteError(w, gobl.ErrInput.WithReason("missing addon key"))
		return
	}

	d, err := ops.AddonData(key)
	if err != nil {
		if errors.Is(err, ops.ErrAddonNotFound) {
			WriteError(w, gobl.ErrNotFound.WithReason("addon not found"))
			return
		}
		WriteError(w, gobl.ErrInternal.WithCause(err))
		return
	}
	WriteRawJSON(w, d)
}
