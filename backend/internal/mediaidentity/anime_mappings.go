package mediaidentity

// Anime-Lists maps each catalog through an AniDB episode identity. Do not
// derive season boundaries from air dates or assume a fixed cour length.
import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

type animeList struct {
	XMLName xml.Name     `xml:"anime-list"`
	Entries []animeEntry `xml:"anime"`
}
type animeEntry struct {
	ID       string      `xml:"anidbid,attr"`
	TVDB     string      `xml:"tvdbid,attr"`
	TMDB     string      `xml:"tmdbtv,attr"`
	TVSeason string      `xml:"defaulttvdbseason,attr"`
	TMSeason string      `xml:"tmdbseason,attr"`
	TVOffset string      `xml:"episodeoffset,attr"`
	TMOffset string      `xml:"tmdboffset,attr"`
	Rules    []animeRule `xml:"mapping-list>mapping"`
}
type animeRule struct {
	AniSeason int    `xml:"anidbseason,attr"`
	TVSeason  string `xml:"tvdbseason,attr"`
	TMSeason  string `xml:"tmdbseason,attr"`
	Start     int    `xml:"start,attr"`
	End       int    `xml:"end,attr"`
	Offset    int    `xml:"offset,attr"`
	Pairs     string `xml:",chardata"`
}
type animeIndex map[string][]animeEntry

func parseAnimeMappings(body []byte) (any, error) {
	var list animeList
	if err := xml.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if len(list.Entries) == 0 {
		return nil, fmt.Errorf("empty anime mapping dataset")
	}
	idx := animeIndex{}
	for _, e := range list.Entries {
		if !positiveID(e.TMDB) || !positiveID(e.TVDB) {
			continue
		}
		idx["tmdb:tv:"+e.TMDB] = append(idx["tmdb:tv:"+e.TMDB], e)
		idx["tvdb:series:"+e.TVDB] = append(idx["tvdb:series:"+e.TVDB], e)
	}
	if len(idx) == 0 {
		return nil, fmt.Errorf("anime dataset has no usable series mappings")
	}
	return idx, nil
}
func positiveID(s string) bool         { n, err := strconv.ParseInt(s, 10, 64); return err == nil && n > 0 }
func seasonValue(s string) (int, bool) { n, err := strconv.Atoi(s); return n, err == nil && n >= 0 }
func (e animeEntry) defaults(tmdb bool) (season, offset int, ok bool) {
	s, o := e.TVSeason, e.TVOffset
	if tmdb {
		s, o = e.TMSeason, e.TMOffset
	}
	season, ok = seasonValue(s)
	if o != "" {
		var err error
		offset, err = strconv.Atoi(o)
		ok = ok && err == nil
	}
	return
}
func (r animeRule) season(tmdb bool) (int, bool) {
	if tmdb {
		return seasonValue(r.TMSeason)
	}
	return seasonValue(r.TVSeason)
}

// A mapping to zero or a combined episode remains explicit: it must suppress
// the default even though it cannot be used as a whole-episode alias.
func (r animeRule) pairs() map[int][]int {
	out := map[int][]int{}
	for _, p := range strings.Split(strings.TrimSpace(r.Pairs), ";") {
		parts := strings.SplitN(strings.TrimSpace(p), "-", 2)
		if len(parts) != 2 {
			continue
		}
		a, err := strconv.Atoi(parts[0])
		if err != nil || a <= 0 {
			continue
		}
		var targets []int
		for _, s := range strings.Split(parts[1], "+") {
			n, err := strconv.Atoi(s)
			if err != nil {
				targets = []int{0}
				break
			}
			targets = append(targets, n)
		}
		out[a] = append(out[a], targets...)
	}
	return out
}
func (e animeEntry) forward(tmdb bool, ani EpisodeCoordinate) []EpisodeCoordinate {
	var explicit, ranges []EpisodeCoordinate
	explicitFound := false
	for _, r := range e.Rules {
		s, ok := r.season(tmdb)
		if !ok || r.AniSeason != ani.Season {
			continue
		}
		if targets, ok := r.pairs()[ani.Episode]; ok {
			explicitFound = true
			for _, n := range targets {
				explicit = append(explicit, EpisodeCoordinate{s, n})
			}
		}
		if r.Start > 0 && r.End >= r.Start && ani.Episode >= r.Start && ani.Episode <= r.End {
			ranges = append(ranges, EpisodeCoordinate{s, ani.Episode + r.Offset})
		}
	}
	if explicitFound {
		return uniqueCoordinates(explicit)
	}
	if len(ranges) > 0 {
		return uniqueCoordinates(ranges)
	}
	s, o, ok := e.defaults(tmdb)
	if ok && ani.Season == 1 {
		return []EpisodeCoordinate{{s, ani.Episode + o}}
	}
	return nil
}
func uniqueCoordinates(in []EpisodeCoordinate) []EpisodeCoordinate {
	out := []EpisodeCoordinate{}
	for _, c := range in {
		found := false
		for _, v := range out {
			if v == c {
				found = true
				break
			}
		}
		if !found {
			out = append(out, c)
		}
	}
	return out
}
func (e animeEntry) inverse(tmdb bool, c EpisodeCoordinate) []EpisodeCoordinate {
	var candidates []EpisodeCoordinate
	s, o, ok := e.defaults(tmdb)
	if ok && c.Season == s && c.Episode-o > 0 {
		candidates = append(candidates, EpisodeCoordinate{1, c.Episode - o})
	}
	for _, r := range e.Rules {
		s, ok := r.season(tmdb)
		if !ok || s != c.Season {
			continue
		}
		n := c.Episode - r.Offset
		if r.Start > 0 && n >= r.Start && n <= r.End {
			candidates = append(candidates, EpisodeCoordinate{r.AniSeason, n})
		}
		for a, targets := range r.pairs() {
			for _, n := range targets {
				if n == c.Episode {
					candidates = append(candidates, EpisodeCoordinate{r.AniSeason, a})
				}
			}
		}
	}
	var out []EpisodeCoordinate
	for _, a := range uniqueCoordinates(candidates) {
		for _, target := range e.forward(tmdb, a) {
			if target == c {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// resolve requires a bijection. Split/combined episodes need segment playback,
// not an alternate whole-file identity, and are deliberately excluded.
func (idx animeIndex) resolve(titleID string, c EpisodeCoordinate) (int64, EpisodeCoordinate, bool) {
	return idx.resolveTo(titleID, c, false)
}

func (idx animeIndex) resolveTo(titleID string, c EpisodeCoordinate, targetTMDB bool) (int64, EpisodeCoordinate, bool) {
	tmdb := strings.HasPrefix(titleID, "tmdb:tv:")
	bestRank := -1
	var chosen EpisodeCoordinate
	var tvdb int64
	ambiguous := false
	for _, e := range idx[titleID] {
		inv := e.inverse(tmdb, c)
		if len(inv) == 0 {
			continue
		}
		// Later cour offsets bound earlier open-ended default mappings.
		rank := 0
		s, o, ok := e.defaults(tmdb)
		if ok && s == c.Season && o >= 0 {
			rank = o + 1
		}
		for _, r := range e.Rules {
			rs, ok := r.season(tmdb)
			if ok && rs == c.Season {
				for _, a := range inv {
					if r.AniSeason == a.Season {
						if _, ok := r.pairs()[a.Episode]; ok {
							rank = 100000
						}
						if r.Start > 0 && a.Episode >= r.Start && a.Episode <= r.End {
							rank = 100000
						}
					}
				}
			}
		}
		if rank < bestRank {
			continue
		}
		valid := len(inv) == 1
		var target EpisodeCoordinate
		if valid {
			from, to := e.forward(tmdb, inv[0]), e.forward(targetTMDB, inv[0])
			valid = len(from) == 1 && len(to) == 1 && to[0].Episode > 0
			if valid {
				target = to[0]
				valid = len(e.inverse(targetTMDB, target)) == 1
			}
		}
		targetID := e.TVDB
		if targetTMDB {
			targetID = e.TMDB
		}
		id, _ := strconv.ParseInt(targetID, 10, 64)
		if rank > bestRank {
			bestRank = rank
			tvdb = id
			chosen = target
			ambiguous = !valid
		} else if !valid || id != tvdb || target != chosen {
			ambiguous = true
		}
	}
	return tvdb, chosen, bestRank >= 0 && !ambiguous
}
