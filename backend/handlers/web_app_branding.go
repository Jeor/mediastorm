package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"os"
	"time"
)

// Keep the exported manifest's metadata and default icons, but resolve uploaded
// branding on each request so changing or removing an icon needs no new build.
func (h *WebAppHandler) serveManifest(w http.ResponseWriter, r *http.Request, filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil || manifest == nil {
		http.Error(w, "invalid web app manifest", http.StatusInternalServerError)
		return
	}
	if h.Branding != nil {
		if settings, err := h.Branding.Manager.Load(); err == nil {
			if iconPath, err := brandingImagePath(settings, brandingSlots["web-icon"]); err == nil {
				if icon, err := os.Open(iconPath); err == nil {
					config, format, err := image.DecodeConfig(icon)
					icon.Close()
					if err == nil {
						manifest["icons"] = []map[string]string{{
							"src":   webUIBrandingURL(settings, settings.Server.BasePath, "web-icon", "favicon-32.png"),
							"sizes": fmt.Sprintf("%dx%d", config.Width, config.Height),
							"type":  "image/" + format,
						}}
					}
				}
			}
		}
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		http.Error(w, "invalid web app manifest", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/manifest+json")
	http.ServeContent(w, r, "manifest.json", time.Time{}, bytes.NewReader(data))
}
