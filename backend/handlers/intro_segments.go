package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var introDBSegmentsURL = "https://api.introdb.app/segments"
var skipDBSegmentsURL = "https://api.skipdb.tv/api/segments"

var introDBIMDbPattern = regexp.MustCompile(`^tt[0-9]+$`)

type introDBSegment struct {
	StartMS         *int64  `json:"start_ms"`
	EndMS           *int64  `json:"end_ms"`
	Confidence      float64 `json:"confidence"`
	SubmissionCount int     `json:"submission_count"`
	Source          string  `json:"source,omitempty"`
}

type skipDBSegmentsResponse struct {
	Segments struct {
		Intro *skipDBSegment `json:"intro"`
		Recap *skipDBSegment `json:"recap"`
		Outro *skipDBSegment `json:"outro"`
	} `json:"segments"`
}

type skipDBSegment struct {
	StartMS *int64 `json:"start_ms"`
	EndMS   *int64 `json:"end_ms"`
	Match   string `json:"match"`
}

type introDBSegmentsResponse struct {
	IMDbID  string          `json:"imdb_id,omitempty"`
	Season  int             `json:"season,omitempty"`
	Episode int             `json:"episode,omitempty"`
	Intro   *introDBSegment `json:"intro"`
	Recap   *introDBSegment `json:"recap"`
	Outro   *introDBSegment `json:"outro"`
}

type cachedIntroDBSegments struct {
	response  introDBSegmentsResponse
	expiresAt time.Time
}

var webIntroDBCache = struct {
	sync.RWMutex
	entries map[string]cachedIntroDBSegments
}{entries: make(map[string]cachedIntroDBSegments)}

var webIntroDBHTTPClient = &http.Client{Timeout: 6 * time.Second}

// GetIntroSegments resolves IntroDB, then SkipDB, for the standalone web player.
// The web client uses this same-origin bridge because IntroDB restricts browser origins.
func (h *VideoHandler) GetIntroSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.HandleOptions(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	imdbID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("imdbId")))
	season, seasonErr := strconv.Atoi(r.URL.Query().Get("season"))
	episode, episodeErr := strconv.Atoi(r.URL.Query().Get("episode"))
	if !introDBIMDbPattern.MatchString(imdbID) || seasonErr != nil || episodeErr != nil || season <= 0 || episode <= 0 {
		http.Error(w, "valid imdbId, season, and episode are required", http.StatusBadRequest)
		return
	}
	duration, _ := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 {
		duration = 0
	}
	duration = math.Round(duration)

	cacheKey := fmt.Sprintf("%s:%d:%d:%.0f", imdbID, season, episode, duration)
	if cached, ok := getCachedIntroDBSegments(cacheKey); ok {
		writeIntroDBSegments(w, cached)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result, introErr := fetchIntroDBSegments(ctx, imdbID, season, episode)
	result.IMDbID = imdbID
	result.Season = season
	result.Episode = episode
	result.Intro = validIntroDBSegment(result.Intro, duration)
	result.Recap = validIntroDBSegment(result.Recap, duration)
	result.Outro = validIntroDBSegment(result.Outro, duration)
	if result.Intro == nil || result.Recap == nil || result.Outro == nil {
		skip, skipErr := fetchSkipDBSegments(ctx, imdbID, season, episode, duration)
		if introErr != nil && skipErr != nil {
			http.Error(w, "intro segments unavailable", http.StatusBadGateway)
			return
		}
		if skipErr == nil {
			if result.Intro == nil {
				result.Intro = validSkipDBSegment(skip.Segments.Intro, duration)
			}
			if result.Recap == nil {
				result.Recap = validSkipDBSegment(skip.Segments.Recap, duration)
			}
			if result.Outro == nil {
				result.Outro = validSkipDBSegment(skip.Segments.Outro, duration)
			}
		}
	}
	ttl := 30 * time.Minute
	if result.Intro != nil || result.Recap != nil || result.Outro != nil {
		ttl = 6 * time.Hour
	}
	cacheIntroDBSegments(cacheKey, result, ttl)
	writeIntroDBSegments(w, result)
}

func fetchIntroDBSegments(ctx context.Context, imdbID string, season, episode int) (introDBSegmentsResponse, error) {
	query := url.Values{"imdb_id": {imdbID}, "season": {strconv.Itoa(season)}, "episode": {strconv.Itoa(episode)}}
	var result introDBSegmentsResponse
	err := fetchSegmentJSON(ctx, introDBSegmentsURL+"?"+query.Encode(), &result)
	return result, err
}

func fetchSkipDBSegments(ctx context.Context, imdbID string, season, episode int, duration float64) (skipDBSegmentsResponse, error) {
	query := url.Values{"imdb_id": {imdbID}, "season": {strconv.Itoa(season)}, "episode": {strconv.Itoa(episode)}}
	if duration > 0 {
		query.Set("duration", strconv.FormatFloat(duration, 'f', 0, 64))
	}
	var result skipDBSegmentsResponse
	err := fetchSegmentJSON(ctx, skipDBSegmentsURL+"?"+query.Encode(), &result)
	return result, err
}

func fetchSegmentJSON(ctx context.Context, endpoint string, destination any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MediaStorm-WebPlayer/1.0")
	resp, err := webIntroDBHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("segment provider returned %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(destination)
}

func validSkipDBSegment(segment *skipDBSegment, duration float64) *introDBSegment {
	if segment == nil || segment.Match == "out-of-range" || segment.StartMS == nil || segment.EndMS == nil || *segment.StartMS < 0 || *segment.EndMS <= *segment.StartMS {
		return nil
	}
	end := *segment.EndMS
	if duration > 0 {
		durationMS := int64(duration * 1000)
		if *segment.StartMS >= durationMS {
			return nil
		}
		if end > durationMS {
			end = durationMS
		}
	}
	return &introDBSegment{StartMS: segment.StartMS, EndMS: &end, Source: "skipdb"}
}

func validIntroDBSegment(segment *introDBSegment, duration float64) *introDBSegment {
	if segment == nil || segment.StartMS == nil || segment.EndMS == nil || *segment.StartMS < 0 || *segment.EndMS <= *segment.StartMS {
		return nil
	}
	if duration > 0 {
		durationMS := int64(duration * 1000)
		if *segment.StartMS >= durationMS {
			return nil
		}
		if *segment.EndMS > durationMS {
			end := durationMS
			segment.EndMS = &end
		}
	}
	return segment
}

func getCachedIntroDBSegments(key string) (introDBSegmentsResponse, bool) {
	webIntroDBCache.RLock()
	entry, ok := webIntroDBCache.entries[key]
	webIntroDBCache.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			webIntroDBCache.Lock()
			delete(webIntroDBCache.entries, key)
			webIntroDBCache.Unlock()
		}
		return introDBSegmentsResponse{}, false
	}
	return entry.response, true
}

func cacheIntroDBSegments(key string, response introDBSegmentsResponse, ttl time.Duration) {
	webIntroDBCache.Lock()
	webIntroDBCache.entries[key] = cachedIntroDBSegments{response: response, expiresAt: time.Now().Add(ttl)}
	webIntroDBCache.Unlock()
}

func writeIntroDBSegments(w http.ResponseWriter, response introDBSegmentsResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_ = json.NewEncoder(w).Encode(response)
}
