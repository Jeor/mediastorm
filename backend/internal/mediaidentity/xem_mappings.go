package mediaidentity

import (
	"encoding/json"
	"fmt"
	"strings"
)

type xemEpisode struct {
	Season   int `json:"season"`
	Episode  int `json:"episode"`
	Absolute int `json:"absolute"`
}
type xemTarget struct {
	Coordinate EpisodeCoordinate
	Absolute   int
}
type xemIndex map[EpisodeCoordinate]xemTarget

func parseXEMMappings(body []byte) (any, error) {
	var payload struct {
		Result  string          `json:"result"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Result == "failure" {
		if strings.Contains(payload.Message, "no single connection") || strings.Contains(payload.Message, "no show with the tvdb_id") {
			return xemIndex{}, nil
		}
		return nil, fmt.Errorf("XEM failure")
	}
	if payload.Result != "success" {
		return nil, fmt.Errorf("missing XEM success result")
	}
	var rows []struct {
		TVDB  *xemEpisode `json:"tvdb"`
		Scene *xemEpisode `json:"scene"`
	}
	if err := json.Unmarshal(payload.Data, &rows); err != nil {
		return nil, err
	}
	out := xemIndex{}
	conflicts := map[EpisodeCoordinate]bool{}
	reverse := map[EpisodeCoordinate]EpisodeCoordinate{}
	for _, row := range rows {
		if row.TVDB == nil || row.Scene == nil || row.TVDB.Episode <= 0 || row.Scene.Episode <= 0 {
			continue
		}
		a, b := EpisodeCoordinate{row.TVDB.Season, row.TVDB.Episode}, EpisodeCoordinate{row.Scene.Season, row.Scene.Episode}
		if prior, ok := out[a]; ok && (prior.Coordinate != b || prior.Absolute != row.Scene.Absolute) {
			conflicts[a] = true
		}
		if prior, ok := reverse[b]; ok && prior != a {
			conflicts[a] = true
			conflicts[prior] = true
		}
		out[a] = xemTarget{b, row.Scene.Absolute}
		reverse[b] = a
	}
	for a := range conflicts {
		delete(out, a)
	}
	return out, nil
}
