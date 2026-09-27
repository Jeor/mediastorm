package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

func homeViewMediaType(view string) string {
	switch view {
	case "movies":
		return "movie"
	case "shows":
		return "series"
	}
	return ""
}

func matchesHomeView(mediaType, view string) bool {
	switch strings.ToLower(mediaType) {
	case "movie", "movies", "film", "films":
		return view == "movies"
	case "series", "tv", "show", "shows", "episode":
		return view == "shows"
	}
	return false
}

// getHomeView filters the source index before taking the requested page. Some
// provider handlers understand filterMediaType; the final guard also covers
// providers that do not. Discovery uses the existing 500-title browse window.
func (h *DisplayListHandler) getHomeView(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("homeView")
	limit, offset := parseLimitOffset(r)
	query := r.URL.Query()
	query.Set("filterMediaType", homeViewMediaType(view))
	if query.Get("source") == "top-ten" {
		query.Set("mediaType", homeViewMediaType(view))
	}
	query.Set("offset", "0")
	query.Set("limit", strconv.Itoa(maxDiscoveryListItems))
	switch query.Get("source") {
	case "", "watchlist", "custom-list", "custom_user_list", "custom-user-list", "permanent-prequeue":
		query.Set("limit", "0")
	}
	delegated := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = query.Encode()
	delegated.URL = &u
	rec := httptest.NewRecorder()
	h.get(rec, delegated)
	if rec.Code >= 400 {
		for k, values := range rec.Header() {
			w.Header()[k] = values
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		http.Error(w, "invalid home view source response", http.StatusBadGateway)
		return
	}
	items, _ := payload["items"].([]interface{})
	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		title := object
		if nested, ok := object["title"].(map[string]interface{}); ok {
			title = nested
		}
		mediaType, _ := title["mediaType"].(string)
		if mediaType == "" && object["seriesId"] != nil {
			mediaType = "movie"
			if object["nextEpisode"] != nil {
				mediaType = "series"
			}
		}
		if matchesHomeView(mediaType, view) {
			filtered = append(filtered, item)
		}
	}
	payload["total"] = len(filtered)
	payload["unfilteredTotal"] = len(filtered)
	if offset > len(filtered) {
		offset = len(filtered)
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}
	payload["items"] = filtered
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
