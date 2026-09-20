package handlers

import (
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"novastream/config"
)

func TestWebAppManifestBrandingLifecycle(t *testing.T) {
	root := writeWebAppFixture(t)
	original := `{"name":"custom app","start_url":"/watch/","icons":[{"src":"/watch/default.png","sizes":"512x512","type":"image/png"}]}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings()
	settings.Cache.Directory = t.TempDir()
	settings.Server.BasePath = "/mediastorm"
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	handler := NewWebAppHandler(root, "/watch")
	handler.Branding = NewSettingsHandler(manager)
	dir := brandingImageDir(settings)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	check := func(wantSrc, wantSize, wantType string) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/watch/manifest.json?v=branding", nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Type") != "application/manifest+json" {
			t.Fatalf("unexpected response: %d %v", rec.Code, rec.Header())
		}
		var manifest struct {
			Name  string `json:"name"`
			Icons []struct {
				Src   string `json:"src"`
				Sizes string `json:"sizes"`
				Type  string `json:"type"`
			} `json:"icons"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != "custom app" || len(manifest.Icons) != 1 {
			t.Fatalf("unexpected manifest: %+v", manifest)
		}
		icon := manifest.Icons[0]
		if icon.Src != wantSrc || icon.Sizes != wantSize || icon.Type != wantType {
			t.Fatalf("unexpected icon: %+v", icon)
		}
	}
	check("/watch/default.png", "512x512", "image/png")
	for _, format := range []string{"png", "jpeg"} {
		ext := format
		if format == "jpeg" {
			ext = "jpg"
		}
		iconPath := filepath.Join(dir, "web-icon."+ext)
		f, err := os.Create(iconPath)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 512, 512))
		if format == "png" {
			err = png.Encode(f, img)
		} else {
			err = jpeg.Encode(f, img, nil)
		}
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(iconPath)
		if err != nil {
			t.Fatal(err)
		}
		check(fmt.Sprintf("/mediastorm/api/branding/images/web-icon?v=%d", info.ModTime().Unix()), "512x512", "image/"+format)
		for _, serve := range []http.HandlerFunc{handler.Branding.ServeWebIcon, handler.Branding.ServeAppleTouchIcon} {
			rec := httptest.NewRecorder()
			serve(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/"+format {
				t.Fatalf("icon response: %d %v", rec.Code, rec.Header())
			}
		}
		if err := os.Remove(iconPath); err != nil {
			t.Fatal(err)
		}
	}
	check("/watch/default.png", "512x512", "image/png")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/watch/manifest.json", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: %d, %s", rec.Code, rec.Body.String())
	}
}
