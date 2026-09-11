package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"net/url"
	"novastream/config"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var seekrHTTPTransport http.RoundTripper

var seekrID = regexp.MustCompile(`^tt[0-9]+$`)
var seekrCue = regexp.MustCompile(`(?m)(\d+:\d{2}:\d{2}\.\d{3})\s+-->[^\n]+\n([^\r\n]+)#xywh=(\d+),(\d+),(\d+),(\d+)`)

// Only the API origin receives credentials. Signed sheet URLs remain server-side.
func seekrFetch(ctx context.Context, rawURL, key string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || (u.Host != "sprites.seekr.tv" && u.Host != "api.seekr.tv") || u.User != nil {
		return nil, fmt.Errorf("invalid Seekr URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MediaStorm/1.0")
	if key != "" {
		if u.Host != "api.seekr.tv" {
			return nil, fmt.Errorf("invalid Seekr credential origin")
		}
		req.Header.Set("X-API-Key", strings.TrimSpace(key))
	}
	client := &http.Client{Transport: seekrHTTPTransport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Seekr request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Seekr status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("Seekr response too large")
	}
	return data, err
}

func seekrQuery(input url.Values, duration float64) (url.Values, error) {
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) || duration > 86400 {
		return nil, fmt.Errorf("invalid duration")
	}
	q := url.Values{"duration_ms": {strconv.FormatInt(int64(duration*1000), 10)}}
	prefix := ""
	switch input.Get("mediaType") {
	case "episode":
		prefix = "show_"
		season, e1 := strconv.Atoi(input.Get("season"))
		episode, e2 := strconv.Atoi(input.Get("episode"))
		if e1 != nil || e2 != nil || season < 0 || episode < 1 {
			return nil, fmt.Errorf("missing episode identity")
		}
		q.Set("season", strconv.Itoa(season))
		q.Set("episode", strconv.Itoa(episode))
	case "movie":
	default:
		return nil, fmt.Errorf("unsupported media type")
	}
	if id := input.Get("imdbId"); seekrID.MatchString(id) {
		q.Set(prefix+"imdb_id", id)
	} else if id, err := strconv.Atoi(input.Get("tmdbId")); err == nil && id > 0 {
		q.Set(prefix+"tmdb_id", strconv.Itoa(id))
	} else {
		return nil, fmt.Errorf("missing title identity")
	}
	return q, nil
}

func (m *ThumbnailManager) loadSeekr(path string, duration float64, input url.Values, apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("missing Seekr key")
	}
	q, err := seekrQuery(input, duration)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := seekrFetch(ctx, "https://api.seekr.tv/sprites?"+q.Encode(), apiKey, 1<<20)
	if err != nil {
		return err
	}
	var response struct {
		VTTURL string `json:"vtt_url"`
	}
	if err = json.Unmarshal(data, &response); err != nil || response.VTTURL == "" {
		return fmt.Errorf("missing Seekr manifest")
	}
	base, _ := url.Parse("https://sprites.seekr.tv")
	ref, err := url.Parse(response.VTTURL)
	if err != nil {
		return err
	}
	vttURL := base.ResolveReference(ref)
	vttQuery := vttURL.Query()
	vttQuery.Set("st", "1")
	vttURL.RawQuery = vttQuery.Encode()
	data, err = seekrFetch(ctx, vttURL.String(), "", 2<<20)
	if err != nil {
		return err
	}
	cues := seekrCue.FindAllStringSubmatch(strings.ReplaceAll(string(data), "\r\n", "\n"), -1)
	if len(cues) == 0 || len(cues) > 10000 {
		return fmt.Errorf("invalid Seekr cues")
	}
	key := thumbnailKey(path)
	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(m.baseDir, "seekr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	manifest := &thumbnailManifest{Key: key, PathHash: key, Status: "generating", Phase: "seekr", DurationSec: duration, IntervalSec: 1, FilterVer: thumbnailFilterVersion}
	destination := filepath.Join(m.baseDir, key)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	manifest.Total = len(cues)
	publish := func() error {
		manifest.Generated = len(manifest.Thumbnails)
		return m.writeManifest(manifest)
	}
	if err := publish(); err != nil {
		return err
	}
	var sheetURL string
	var sheet image.Image
	for i, cue := range cues {
		if err := ctx.Err(); err != nil {
			return err
		}
		parts := strings.Split(cue[1], ":")
		hh, _ := strconv.Atoi(parts[0])
		mm, _ := strconv.Atoi(parts[1])
		ss, _ := strconv.ParseFloat(parts[2], 64)
		timestamp := float64(hh*3600+mm*60) + ss
		if timestamp >= duration {
			continue
		}
		ref, err := url.Parse(cue[2])
		if err != nil {
			return err
		}
		target := vttURL.ResolveReference(ref).String()
		if target != sheetURL {
			if len(manifest.Thumbnails) > 0 {
				if err := publish(); err != nil {
					return err
				}
			}
			data, err = seekrFetch(ctx, target, "", 16<<20)
			if err != nil {
				return err
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || cfg.Width*cfg.Height > 40000000 {
				return fmt.Errorf("invalid Seekr sheet")
			}
			sheet, err = jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				return err
			}
			sheetURL = target
		}
		coords := make([]int, 4)
		for j := range coords {
			coords[j], err = strconv.Atoi(cue[j+3])
			if err != nil {
				return err
			}
		}
		rect := image.Rect(coords[0], coords[1], coords[0]+coords[2], coords[1]+coords[3])
		if rect.Empty() || !rect.In(sheet.Bounds()) {
			return fmt.Errorf("invalid Seekr tile")
		}
		cropper, ok := sheet.(interface {
			SubImage(image.Rectangle) image.Image
		})
		if !ok {
			return fmt.Errorf("unsupported Seekr image")
		}
		filename := fmt.Sprintf("seekr-%05d.jpg", i)
		f, err := os.Create(filepath.Join(dir, filename))
		if err != nil {
			return err
		}
		err = jpeg.Encode(f, cropper.SubImage(rect), &jpeg.Options{Quality: 90})
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		// Publish only closed, atomically moved JPEGs so native image loaders
		// never observe a partially written tile.
		if err := os.Rename(filepath.Join(dir, filename), filepath.Join(destination, filename)); err != nil {
			return err
		}
		manifest.Thumbnails = append(manifest.Thumbnails, thumbnailDetails{TimeSec: timestamp, File: filename})
		if len(manifest.Thumbnails) == 1 || len(manifest.Thumbnails)%25 == 0 {
			if err := publish(); err != nil {
				return err
			}
		}
	}
	if len(manifest.Thumbnails) == 0 {
		return fmt.Errorf("no Seekr tiles")
	}
	manifest.Status = "ready"
	manifest.Total = len(manifest.Thumbnails)
	return publish()
}

// This path never resolves or reads the playback source, including on an API miss.
func (m *ThumbnailManager) startSeekrOnly(path string, duration float64, query url.Values, settings config.PlaybackThumbnailSettings) (string, bool) {
	key := thumbnailKey(path)
	if !settings.SeekrEnabled || strings.TrimSpace(settings.SeekrAPIKey) == "" {
		return key, false
	}
	if _, err := seekrQuery(query, duration); err != nil {
		return key, false
	}
	if manifest, err := m.readManifest(key); err == nil && manifest.Status == "ready" && m.manifestFilesComplete(manifest) {
		return key, false
	}
	if m.seekrRecentlyUnavailable(path) {
		return key, false
	}
	m.mu.Lock()
	if _, exists := m.inFlight[key]; exists {
		m.mu.Unlock()
		return key, false
	}
	m.inFlight[key] = struct{}{}
	m.mu.Unlock()
	go func() {
		defer func() { m.mu.Lock(); delete(m.inFlight, key); m.mu.Unlock() }()
		if err := m.loadSeekr(path, duration, query, settings.SeekrAPIKey); err != nil {
			_ = m.writeManifest(&thumbnailManifest{Key: key, PathHash: key, Status: "pending", Phase: "seekr-miss", DurationSec: duration})
		}
	}()
	return key, true
}

func (m *ThumbnailManager) seekrRecentlyUnavailable(path string) bool {
	manifest, err := m.readManifest(thumbnailKey(path))
	return err == nil && manifest.Phase == "seekr-miss" && time.Since(manifest.UpdatedAt) < 5*time.Minute
}
