package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"novastream/prerollassets"

	"github.com/gorilla/mux"
)

const prerollMaxFileSize = 100 << 20

var prerollAssetIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type PrerollHandler struct {
	dir          string
	defaultID    string
	defaultVideo []byte
}

type prerollAssetResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

func NewPrerollHandler(cacheDir string) *PrerollHandler {
	sum := sha256.Sum256(prerollassets.DefaultVideo)
	return &PrerollHandler{
		dir:          filepath.Join(cacheDir, "preroll"),
		defaultID:    hex.EncodeToString(sum[:]),
		defaultVideo: prerollassets.DefaultVideo,
	}
}

func (h *PrerollHandler) Default(w http.ResponseWriter, r *http.Request) {
	h.serveBytes(w, r, h.defaultID, h.defaultVideo)
}

func (h *PrerollHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, prerollMaxFileSize)
	if err := r.ParseMultipartForm(prerollMaxFileSize); err != nil {
		http.Error(w, "invalid video upload (maximum 100 MB)", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "video file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp(h.dirParent(), "preroll-upload-*")
	if err != nil {
		http.Error(w, "unable to create upload", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	hash := sha256.New()
	limited := io.LimitReader(file, prerollMaxFileSize+1)
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), limited)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		http.Error(w, "unable to store upload", http.StatusInternalServerError)
		return
	}
	if size > prerollMaxFileSize {
		http.Error(w, "video exceeds 100 MB", http.StatusRequestEntityTooLarge)
		return
	}

	probe, err := os.Open(tmpPath)
	if err != nil {
		http.Error(w, "unable to validate upload", http.StatusInternalServerError)
		return
	}
	header := make([]byte, 512)
	n, _ := probe.Read(header)
	probe.Close()
	contentType := http.DetectContentType(header[:n])
	if n < 12 || !bytes.Equal(header[4:8], []byte("ftyp")) {
		http.Error(w, fmt.Sprintf("unsupported video type %q; upload an MP4", contentType), http.StatusBadRequest)
		return
	}

	id := hex.EncodeToString(hash.Sum(nil))
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		http.Error(w, "unable to create preroll storage", http.StatusInternalServerError)
		return
	}
	destination := h.assetPath(id)
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if err := os.Rename(tmpPath, destination); err != nil {
			http.Error(w, "unable to save preroll", http.StatusInternalServerError)
			return
		}
	}
	h.writeAssetResponse(w, http.StatusCreated, id, size)
}

func (h *PrerollHandler) Serve(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(mux.Vars(r)["assetID"]))
	if id == h.defaultID {
		h.serveBytes(w, r, id, h.defaultVideo)
		return
	}
	if !prerollAssetIDPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	path := h.assetPath(id)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+id+`"`)
	http.ServeFile(w, r, path)
}

func (h *PrerollHandler) Manifest(w http.ResponseWriter, _ *http.Request) {
	h.writeAssetResponse(w, http.StatusOK, h.defaultID, int64(len(h.defaultVideo)))
}

func (h *PrerollHandler) Options(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *PrerollHandler) dirParent() string {
	parent := filepath.Dir(h.dir)
	_ = os.MkdirAll(parent, 0o755)
	return parent
}

func (h *PrerollHandler) assetPath(id string) string {
	return filepath.Join(h.dir, id+".mp4")
}

func (h *PrerollHandler) serveBytes(w http.ResponseWriter, r *http.Request, id string, data []byte) {
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+id+`"`)
	http.ServeContent(w, r, "preroll.mp4", time.Time{}, bytes.NewReader(data))
}

func (h *PrerollHandler) writeAssetResponse(w http.ResponseWriter, status int, id string, size int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(prerollAssetResponse{
		ID: id, URL: "/api/preroll/assets/" + id, ContentType: "video/mp4", Size: size,
	})
}
