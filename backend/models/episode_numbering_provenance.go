package models

import "strings"

// EpisodeNumbering identifies the provider series and ordering that own the
// season/episode coordinates. It is independent of the selected catalog title.
type EpisodeNumbering struct {
	SeriesID string `json:"seriesId"`
	Ordering string `json:"ordering,omitempty"`
}

func SameEpisodeNumbering(a, b *EpisodeNumbering) bool {
	if a == nil || b == nil {
		return a == b
	}
	order := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return "official"
		}
		return s
	}
	return a.SeriesID == b.SeriesID && order(a.Ordering) == order(b.Ordering)
}

// StampEpisodeNumbering also upgrades metadata read from older cache entries.
func StampEpisodeNumbering(details *SeriesDetails, seriesID string) {
	if details == nil {
		return
	}
	ordering := details.ActiveOrdering
	if ordering == "" {
		ordering = "official"
	}
	n := &EpisodeNumbering{SeriesID: seriesID, Ordering: ordering}
	details.Numbering = n
	for i := range details.Seasons {
		for j := range details.Seasons[i].Episodes {
			details.Seasons[i].Episodes[j].Numbering = n
		}
	}
}

// EpisodeNumberingKey binds cached selection hints to their input coordinates.
func EpisodeNumberingKey(n *EpisodeNumbering) string {
	if n == nil {
		return ""
	}
	ordering := strings.ToLower(strings.TrimSpace(n.Ordering))
	if ordering == "" {
		ordering = "official"
	}
	return n.SeriesID + "|" + ordering
}
