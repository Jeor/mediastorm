package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultSpeedTestBytes int64 = 128 * 1024 * 1024
	maxSpeedTestBytes     int64 = 2 * 1024 * 1024 * 1024
	speedTestChunkBytes         = 1024 * 1024
)

func ServeSpeedTest(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	size := defaultSpeedTestBytes
	if raw := r.URL.Query().Get("bytes"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 || parsed > maxSpeedTestBytes {
			http.Error(w, "bytes must be between 0 and 2147483648", http.StatusBadRequest)
			return
		}
		size = parsed
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method == http.MethodHead || size == 0 {
		return
	}

	var sent int64
	sampleID, _ := strconv.ParseInt(r.URL.Query().Get("sampleId"), 10, 64)
	flow, _ := strconv.Atoi(r.URL.Query().Get("flow"))
	streams, _ := strconv.Atoi(r.URL.Query().Get("streams"))
	defer func() {
		log.Printf(
			"[speed-test] complete sampleId=%d flow=%d/%d requestedBytes=%d sentBytes=%d elapsedMs=%d method=%s",
			sampleID,
			flow,
			streams,
			size,
			sent,
			time.Since(startedAt).Milliseconds(),
			r.Method,
		)
	}()

	chunk := make([]byte, speedTestChunkBytes)
	for i := range chunk {
		chunk[i] = byte((i * 31) % 251)
	}

	remaining := size
	for remaining > 0 {
		if err := r.Context().Err(); err != nil {
			return
		}

		toWrite := int64(len(chunk))
		if remaining < toWrite {
			toWrite = remaining
		}
		written, err := w.Write(chunk[:toWrite])
		sent += int64(written)
		if err != nil {
			return
		}
		if written == 0 {
			return
		}
		remaining -= int64(written)
	}
}
