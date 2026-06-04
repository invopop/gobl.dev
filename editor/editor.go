// Package editor provides the GOBL web editor UI using PopUI and Templ.
package editor

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/invopop/gobl.dev/editor/examples"
	popui "github.com/invopop/popui.go"
)

//go:embed assets/*
var editorAssetsEmbed embed.FS

// editorAssets is the sub-filesystem without the "assets" prefix, used
// both for serving files and for generating versioned paths.
var editorAssets fs.FS

func init() {
	editorAssets, _ = fs.Sub(editorAssetsEmbed, "assets")
}

// AssetPath is the URL prefix for serving editor assets.
const AssetPath = "/_editor"

// Handler returns an http.HandlerFunc that renders the editor page.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initial := pickExampleFromAcceptLanguage(r.Header.Get("Accept-Language"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = Page(initial.ID).Render(r.Context(), w)
	}
}

// RegisterAssets registers the editor and popui static asset handlers,
// plus editor-private JSON routes, onto the given ServeMux.
func RegisterAssets(mux *http.ServeMux) {
	// PopUI assets at /_popui/ (embed FS contains assets/ subdirectory)
	mux.Handle(popui.AssetPath+"/", http.StripPrefix(
		popui.AssetPath, http.FileServerFS(popui.Assets),
	))
	// Editor assets at /_editor/
	mux.Handle(AssetPath+"/", http.StripPrefix(
		AssetPath, http.FileServerFS(editorAssets),
	))

	// Editor-private JSON routes.
	mux.HandleFunc("GET /_editor/examples", handleExamplesList)
	mux.HandleFunc("GET /_editor/examples/{id}", handleExampleGet)
	mux.HandleFunc("GET /_editor/formats", handleFormats)
	mux.HandleFunc("POST /_editor/convert", handleConvert)
}

// bootstrap returns the initial state blob rendered into the page as a
// JSON <script> tag and read by the Alpine editor component on boot.
func bootstrap(initialExampleID string) map[string]any {
	return map[string]any{
		"initialExampleID": initialExampleID,
		"examples":         examples.All(),
		"formats":          formatList(),
	}
}

func handleExamplesList(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(examples.All())
}

func handleExampleGet(w http.ResponseWriter, r *http.Request) {
	data, ok := examples.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}
