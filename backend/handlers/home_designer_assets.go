package handlers

import (
	"embed"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

//go:embed admin_assets/home_designer/*.js admin_assets/home_designer/*.css
var homeDesignerAssets embed.FS

var homeDesignerAssetContentTypes = map[string]string{
	"app.js":            "text/javascript; charset=utf-8",
	"api.js":            "text/javascript; charset=utf-8",
	"home_designer.css": "text/css; charset=utf-8",
	"store.js":          "text/javascript; charset=utf-8",
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
