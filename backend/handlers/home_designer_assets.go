package handlers

import (
	"embed"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

//go:embed admin_assets/home_designer/*.js admin_assets/home_designer/*.css
var homeDesignerAssets embed.FS

// homeDesignerAssetVersion is replaced with the source revision in test-image
// builds so every deployment gets a fresh asset path through edge caches.
var homeDesignerAssetVersion = "dev"

var homeDesignerAssetContentTypes = map[string]string{
	"app.js":            "text/javascript; charset=utf-8",
	"api.js":            "text/javascript; charset=utf-8",
	"home_designer.css": "text/css; charset=utf-8",
	"library.js":        "text/javascript; charset=utf-8",
	"outline.js":        "text/javascript; charset=utf-8",
	"preview.js":        "text/javascript; charset=utf-8",
	"store.js":          "text/javascript; charset=utf-8",
	"theme.js":          "text/javascript; charset=utf-8",
	"workspace.js":      "text/javascript; charset=utf-8",
}

// HomeDesignerAsset serves only the small, embedded editor bootstrap assets.
func HomeDesignerAsset(w http.ResponseWriter, r *http.Request) {
	asset := mux.Vars(r)["asset"]
	contentType, allowed := homeDesignerAssetContentTypes[asset]
	if !allowed || strings.ContainsAny(asset, "/\\") {
		http.NotFound(w, r)
		return
	}
	contents, err := homeDesignerAssets.ReadFile("admin_assets/home_designer/" + asset)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(contents)
}
