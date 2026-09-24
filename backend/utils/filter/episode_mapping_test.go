package filter

import (
	"fmt"
	"novastream/internal/mappingtest"
	"novastream/models"
	"testing"
)

func TestAnimeReleaseAliasesFilterAndBindCoordinates(t *testing.T) {
	mappingtest.Install(t)
	cases := []struct {
		name, id     string
		s, e, ms, me int
	}{
		{"Kaiju No 8", "207468", 1, 13, 2, 1},
		{"Frieren", "209867", 1, 29, 2, 1},
		{"Bleach", "30984", 2, 14, 17, 14},
		{"One Piece", "37854", 2, 1, 5, 2},
		{"Jujutsu Kaisen", "95479", 1, 48, 3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "tmdb:tv:" + tc.id
			mappingtest.Warm(t.Context(), id, tc.s, tc.e)
			opts := Options{TitleID: id, ExpectedTitle: tc.name, TargetSeason: tc.s, TargetEpisode: tc.e, TargetAbsoluteEpisode: tc.e, IsAnime: true}
			titles := []string{fmt.Sprintf("%s S%02dE%02d 1080p WEB", tc.name, tc.s, tc.e), fmt.Sprintf("%s S%02dE%02d 1080p WEB", tc.name, tc.ms, tc.me), fmt.Sprintf("%s S%02d 1080p WEB COMPLETE", tc.name, tc.ms), fmt.Sprintf("%s S%02dE%02d 1080p WEB", tc.name, tc.ms, tc.me+1)}
			results := make([]models.NZBResult, len(titles))
			for i, title := range titles {
				results[i] = models.NZBResult{Title: title, Attributes: map[string]string{"targetSeason": fmt.Sprint(tc.s), "targetEpisode": fmt.Sprint(tc.e), "absoluteEpisodeNumber": fmt.Sprint(tc.e)}}
			}
			filtered := Results(results, opts)
			if len(filtered) != 3 {
				t.Fatalf("wanted catalog, alias, pack; got %+v", filtered)
			}
			for i, r := range filtered {
				if i == 0 {
					if r.Attributes["mappedCatalogEpisode"] != "" {
						t.Fatal("catalog release remapped")
					}
					continue
				}
				if r.Attributes["targetSeason"] != fmt.Sprint(tc.ms) || r.Attributes["targetEpisode"] != fmt.Sprint(tc.me) || r.Attributes["absoluteEpisodeNumber"] != "" {
					t.Fatalf("wrong mapped hints: %+v", r.Attributes)
				}
			}
		})
	}
}
func TestAnimeMappingDoesNotRewriteAbsoluteRelease(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	opts := Options{TitleID: "tmdb:tv:207468", ExpectedTitle: "Kaiju No 8", TargetSeason: 1, TargetEpisode: 13, TargetAbsoluteEpisode: 13, IsAnime: true}
	results := Results([]models.NZBResult{{Title: "[SubsPlease] Kaiju No 8 - 13 (1080p) [ABCD1234].mkv"}, {Title: "[SubsPlease] Kaiju No 8 - 01 (1080p) [ABCD1234].mkv"}}, opts)
	if len(results) != 1 || results[0].Attributes["mappedCatalogEpisode"] != "" {
		t.Fatalf("absolute episode handling changed: %+v", results)
	}
	opts.TitleID = "tmdb:tv:99999"
	if got := Results([]models.NZBResult{{Title: "Kaiju No 8 S02E01 1080p WEB"}}, opts); len(got) != 0 {
		t.Fatal("mapping leaked to unrelated ID")
	}
}

func TestMappedResultRevalidationDoesNotLeakAcrossEpisodes(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	opts := Options{TitleID: "tmdb:tv:207468", ExpectedTitle: "Kaiju No 8", TargetSeason: 1, TargetEpisode: 13, TargetAbsoluteEpisode: 13, IsAnime: true}
	original := Results([]models.NZBResult{{Title: "Kaiju No 8 S02 COMPLETE 1080p WEB"}}, opts)
	opts.TargetEpisode = 14
	opts.TargetAbsoluteEpisode = 14
	second := Results(original, opts)
	if len(second) != 1 || second[0].Attributes["targetEpisode"] != "2" {
		t.Fatalf("second episode hints: %+v", second)
	}
	if original[0].Attributes["targetEpisode"] != "1" {
		t.Fatal("filter mutated cached/shared attributes")
	}
}
