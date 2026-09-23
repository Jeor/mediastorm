package mediaidentity

import (
	"context"
	"encoding/json"
	"strconv"
)

type wikidataSnak struct {
	Data struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}
type wikidataStatement struct {
	Rank       string                    `json:"rank"`
	Main       wikidataSnak              `json:"mainsnak"`
	Qualifiers map[string][]wikidataSnak `json:"qualifiers"`
}
type wikidataEntity struct {
	Claims map[string][]wikidataStatement `json:"claims"`
}

func (s wikidataSnak) text() string { var v string; _ = json.Unmarshal(s.Data.Value, &v); return v }
func (e wikidataEntity) singleString(property string) string {
	result := ""
	for _, s := range e.Claims[property] {
		if s.Rank == "deprecated" {
			continue
		}
		v := s.Main.text()
		if v == "" || (result != "" && result != v) {
			return ""
		}
		result = v
	}
	return result
}
func (d *discoveryService) entity(ctx context.Context, id string) (wikidataEntity, error) {
	var payload struct {
		Entities map[string]wikidataEntity `json:"entities"`
	}
	err := d.get(ctx, d.wikidata+"/"+id+".json", &payload)
	return payload.Entities[id], err
}
func (d *discoveryService) parent(ctx context.Context, id, tmdbID string) (string, int, error) {
	entity, err := d.entity(ctx, id)
	if err != nil {
		return "", 0, err
	}
	// Require the season entity to point back to this exact TMDB series.
	if entity.singleString("P4983") != tmdbID && entity.singleString("P12558") != tmdbID {
		return "", 0, nil
	}
	parent := ""
	ordinal := 0
	for _, s := range entity.Claims["P179"] {
		if s.Rank == "deprecated" {
			continue
		}
		var v struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(s.Main.Data.Value, &v)
		q := s.Qualifiers["P1545"]
		if !wikidataIDPattern.MatchString(v.ID) || len(q) != 1 {
			return "", 0, nil
		}
		n, err := strconv.Atoi(q[0].text())
		if err != nil || n < 1 {
			return "", 0, nil
		}
		if parent != "" && (parent != v.ID || ordinal != n) {
			return "", 0, nil
		}
		parent, ordinal = v.ID, n
	}
	return parent, ordinal, nil
}
