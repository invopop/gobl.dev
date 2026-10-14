package api

import (
	"net/http"
	"strings"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
)

// FormatHeader is the response header that names the key of the format an
// export was converted into.
const FormatHeader = "GOBL-Format"

func handleFormatList(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, map[string]any{"formats": ops.Formats()})
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	env, err := ops.Import(r.Context(), &ops.ImportOptions{
		Input:   r.Body,
		Formats: queryFormats(r),
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, env)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	out, err := ops.Export(r.Context(), &ops.ExportOptions{
		ParseOptions: &ops.ParseOptions{
			Input: r.Body,
		},
		Formats: queryFormats(r),
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	mime := out.Format.MIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set(FormatHeader, out.Format.Key.String())
	_, _ = w.Write(out.Data) //nolint:errcheck
}

// queryFormats reads the format keys from the "format" query parameter, which
// may be repeated or hold a comma-separated list, preserving their order.
// Keys contain "+", which query strings decode as a space, so unescaped keys
// such as "ubl+en16931-v2017" are restored.
func queryFormats(r *http.Request) []cbc.Key {
	var keys []cbc.Key
	for _, v := range r.URL.Query()["format"] {
		for _, k := range strings.Split(v, ",") {
			if k = strings.ReplaceAll(strings.TrimSpace(k), " ", "+"); k != "" {
				keys = append(keys, cbc.Key(k))
			}
		}
	}
	return keys
}
