package api

import (
	"net/http"
	"strings"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
)

// FormatHeader is the response header that names the key of the format an
// export was converted into, or an import was detected as.
const FormatHeader = "GOBL-Format"

func handleFormatList(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, map[string]any{"formats": ops.Formats()})
}

// handleFormatCheck reports, for each format that exports the schema of the
// GOBL document in the body, whether it accepts the document.
func handleFormatCheck(w http.ResponseWriter, r *http.Request) {
	checks, err := ops.CheckFormats(r.Context(), &ops.ParseOptions{
		Input: r.Body,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"formats": checks})
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	env, f, err := ops.Import(r.Context(), &ops.ImportOptions{
		Input:   r.Body,
		Formats: queryFormats(r),
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	w.Header().Set(FormatHeader, f.Key.String())
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
