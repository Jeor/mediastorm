package metadata

import (
	"net/url"
	"strings"
)

// resolveStreamingListURL keeps saved copies of retired built-in lists working.
// Only the two known Hulu defaults are replaced; custom lists retain their URLs.
func resolveStreamingListURL(listURL string) string {
	parsed, err := url.Parse(listURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "mdblist.com") {
		return listURL
	}
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/json")
	switch path {
	case "/lists/snoak/top-hulu-movies":
		parsed.Path = "/lists/azodath/top-hulu-movies/json"
	case "/lists/snoak/top-tv-shows-hulu":
		parsed.Path = "/lists/azodath/top-hulu-shows/json"
	default:
		return listURL
	}
	parsed.RawPath = ""
	return parsed.String()
}
