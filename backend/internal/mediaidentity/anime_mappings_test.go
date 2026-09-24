package mediaidentity

import (
	"fmt"
	"novastream/models"
	"os"
	"testing"
)

func animeFixture(t *testing.T) animeIndex {
	t.Helper()
	body, err := os.ReadFile("../mappingtest/testdata/anime-list.xml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := parseAnimeMappings(body)
	if err != nil {
		t.Fatal(err)
	}
	return data.(animeIndex)
}
func TestRealAnimeEpisodeMappings(t *testing.T) {
	idx := animeFixture(t)
	cases := []struct {
		name, id     string
		s, e, ts, te int
	}{
		{"Kaiju first season end", "207468", 1, 12, 1, 12},
		{"Kaiju second season start", "207468", 1, 13, 2, 1},
		{"Kaiju second season end", "207468", 1, 23, 2, 11},
		{"Frieren first cour boundary", "209867", 1, 28, 1, 28},
		{"Frieren second season", "209867", 1, 29, 2, 1},
		{"Frieren third season", "209867", 1, 39, 3, 1},
		{"Bleach first arc end", "30984", 1, 20, 1, 20},
		{"Bleach second arc", "30984", 1, 21, 2, 1},
		{"Bleach last original episode", "30984", 1, 366, 16, 24},
		{"Bleach TYBW", "30984", 2, 1, 17, 1},
		{"Bleach TYBW second cour", "30984", 2, 14, 17, 14},
		{"Bleach TYBW third cour", "30984", 2, 27, 17, 27},
		{"One Piece second arc", "37854", 1, 9, 2, 1},
		{"One Piece season boundary", "37854", 2, 1, 5, 2},
		{"Attack on Titan final season part two", "1429", 4, 17, 4, 17},
		{"Demon Slayer Mugen Train TV", "85937", 2, 1, 2, 1},
		{"Demon Slayer Entertainment District", "85937", 3, 1, 3, 1},
		{"Mushoku second cour", "94664", 1, 12, 1, 12},
		{"Mushoku season two second cour", "94664", 2, 13, 2, 13},
		{"Sword Art Online split cour", "45782", 4, 13, 4, 13},
		{"Jujutsu Culling Game", "95479", 1, 48, 3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tvdb, got, ok := idx.resolve("tmdb:tv:"+tc.id, EpisodeCoordinate{tc.s, tc.e})
			if !ok || got != (EpisodeCoordinate{tc.ts, tc.te}) {
				t.Fatalf("got %+v ok=%v want S%dE%d", got, ok, tc.ts, tc.te)
			}
			id, back, valid := idx.resolveTo(fmt.Sprintf("tvdb:series:%d", tvdb), got, true)
			if !valid || fmt.Sprint(id) != tc.id || back != (EpisodeCoordinate{tc.s, tc.e}) {
				t.Fatalf("reverse got id=%d coord=%+v valid=%v", id, back, valid)
			}
		})
	}
	if _, _, ok := idx.resolve("tmdb:tv:999999", EpisodeCoordinate{1, 13}); ok {
		t.Fatal("unknown series mapped")
	}
	if _, _, ok := idx.resolve("tmdb:tv:37854", EpisodeCoordinate{99, 1}); ok {
		t.Fatal("unknown arc guessed")
	}
}
func TestExplicitAnimeMappingSafety(t *testing.T) {
	cases := []struct {
		name, rules   string
		input, output int
		ok            bool
	}{
		{"explicit overrides offset", `<mapping anidbseason="1" tvdbseason="2">;1-5;5-1;</mapping>`, 1, 5, true},
		{"zero suppresses offset", `<mapping anidbseason="1" tvdbseason="2">;1-0;</mapping>`, 1, 0, false},
		{"split episode", `<mapping anidbseason="1" tvdbseason="2">;1-1+2;</mapping>`, 1, 0, false},
		{"combined episode", `<mapping anidbseason="1" tvdbseason="2">;1-1;2-1;</mapping>`, 1, 0, false},
		{"conflicting rules", `<mapping anidbseason="1" tvdbseason="2">;1-1;</mapping><mapping anidbseason="1" tvdbseason="3">;1-1;</mapping>`, 1, 0, false},
		{"special excluded from regular default", `<mapping anidbseason="0" tvdbseason="0">;1-9;</mapping>`, 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `<anime-list><anime anidbid="1" tmdbtv="123" tmdbseason="1" tvdbid="456" defaulttvdbseason="2"><mapping-list>` + tc.rules + `</mapping-list></anime></anime-list>`
			data, err := parseAnimeMappings([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			_, got, ok := data.(animeIndex).resolve("tmdb:tv:123", EpisodeCoordinate{1, tc.input})
			if ok != tc.ok || ok && got.Episode != tc.output {
				t.Fatalf("got=%+v ok=%v", got, ok)
			}
		})
	}
}
func TestAnimeMappingConflictsAndMalformedDataset(t *testing.T) {
	for _, body := range []string{`<html>error</html>`, `<anime-list/>`, `<anime-list><anime tmdbtv="1" tvdbid="movie"/></anime-list>`} {
		if _, err := parseAnimeMappings([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	data, err := parseAnimeMappings([]byte(`<anime-list><anime tmdbtv="1" tmdbseason="1" tvdbid="2" defaulttvdbseason="2"/><anime tmdbtv="1" tmdbseason="1" tvdbid="2" defaulttvdbseason="3"/></anime-list>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := data.(animeIndex).resolve("tmdb:tv:1", EpisodeCoordinate{1, 1}); ok {
		t.Fatal("conflicting cour maps accepted")
	}
}

// Set STRMR_ANIME_MAPPING_SNAPSHOT to validate a downloaded upstream snapshot
// without making unit tests dependent on the network.
func TestFullAnimeMappingSnapshot(t *testing.T) {
	path := os.Getenv("STRMR_ANIME_MAPPING_SNAPSHOT")
	if path == "" {
		t.Skip("no upstream snapshot supplied")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := parseAnimeMappings(body)
	if err != nil {
		t.Fatal(err)
	}
	idx := data.(animeIndex)
	if len(idx) < 1000 {
		t.Fatalf("unexpectedly small snapshot: %d", len(idx))
	}
	id, ep, ok := idx.resolve("tmdb:tv:207468", EpisodeCoordinate{1, 13})
	if !ok || id != 423075 || ep != (EpisodeCoordinate{2, 1}) {
		t.Fatalf("upstream Kaiju mapping: %d %+v %t", id, ep, ok)
	}
}

func TestReleaseAliasesRespectExplicitNumbering(t *testing.T) {
	s := NewEpisodeMappingService(nil, nil)
	s.entries["anime-lists"] = mappingEntry{data: animeFixture(t)}
	old := SetEpisodeMappingService(s)
	defer SetEpisodeMappingService(old)
	for _, tc := range []struct {
		name, id, order string
		s, e, count     int
	}{
		{"TVDB under TMDB title", "tvdb:series:423075", "official", 2, 1, 1},
		{"TMDB coordinates", "tmdb:tv:207468", "official", 1, 13, 1},
		{"invalid earlier season boundary", "tvdb:series:423075", "official", 1, 13, 0},
		{"DVD order", "tvdb:series:423075", "dvd", 2, 1, 0},
		{"unknown source", "tvdb:series:99999999", "official", 2, 1, 0},
		{"invalid source", "tvdb:series:0", "official", 2, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ReleaseEpisodeAliases("tmdb:tv:207468", tc.s, tc.e, &models.EpisodeNumbering{SeriesID: tc.id, Ordering: tc.order})
			if len(got) != tc.count {
				t.Fatalf("aliases=%+v want %d", got, tc.count)
			}
			if tc.count > 0 && got[0].Season == tc.s && got[0].Episode == tc.e {
				t.Fatal("identity alias")
			}
		})
	}
}
