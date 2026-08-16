package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPrerollManifestAndDefaultAsset(t *testing.T) {
	handler := NewPrerollHandler(t.TempDir())
	recorder := httptest.NewRecorder()
	handler.Manifest(recorder, httptest.NewRequest(http.MethodGet, "/api/preroll/manifest", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("manifest status = %d", recorder.Code)
	}
	var manifest prerollAssetResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "debe216d5dc988e5f22a7fa35c7f92077153b019174a6045d9222d9970ed7244" {
		t.Fatalf("unexpected embedded asset id %q", manifest.ID)
	}
	if manifest.Size <= 0 || manifest.URL == "" {
		t.Fatalf("invalid manifest: %+v", manifest)
	}

	recorder = httptest.NewRecorder()
	handler.Default(recorder, httptest.NewRequest(http.MethodGet, manifest.URL, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("ETag") != `"`+manifest.ID+`"` {
		t.Fatalf("default response status=%d etag=%q", recorder.Code, recorder.Header().Get("ETag"))
	}
}

func TestPrerollUploadStoresContentAddressedMP4(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewPrerollHandler(cacheDir)
	payload := append([]byte{0, 0, 0, 24}, []byte("ftypisom0000test-video")...)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("video", "custom.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/preroll/assets", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var asset prerollAssetResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &asset); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	wantID := hex.EncodeToString(sum[:])
	if asset.ID != wantID {
		t.Fatalf("asset id=%q want=%q", asset.ID, wantID)
	}
	stored := filepath.Join(cacheDir, "preroll", wantID+".mp4")
	got, err := os.ReadFile(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("stored upload differs from payload")
	}
}

func TestPrerollUploadRejectsNonVideo(t *testing.T) {
	handler := NewPrerollHandler(t.TempDir())
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("video", "notes.txt")
	_, _ = part.Write([]byte("not a video"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/preroll/assets", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
