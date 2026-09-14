package sports

import (
	"encoding/json"
	"net/url"
)

// ESPN uses either a URL or {href} for athlete portraits. Invalid optional
// artwork must not fail a score response; never synthesize URLs from names.
type playerHeadshot string

func (h *playerHeadshot) UnmarshalJSON(data []byte) error {
	var value string
	if json.Unmarshal(data, &value) != nil {
		var object struct {
			Href string `json:"href"`
		}
		_ = json.Unmarshal(data, &object)
		value = object.Href
	}
	*h = ""
	if u, err := url.Parse(value); err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil {
		*h = playerHeadshot(value)
	}
	return nil
}
