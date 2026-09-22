package models

import "testing"

func TestSeriesSeasonPremiereYear(t *testing.T) {
	seasons := []SeriesSeason{
		{Number: 1, Episodes: []SeriesEpisode{{EpisodeNumber: 1, AiredDate: "2000-09-01"}}},
		{Number: 3, Episodes: []SeriesEpisode{{EpisodeNumber: 8, AiredDate: "2012-01-01"}, {EpisodeNumber: 1, AiredDate: "2008-09-01"}}},
		{Number: 4, Episodes: []SeriesEpisode{{EpisodeNumber: 2, AiredDate: "2015-09-01"}}},
		{Number: 5, Episodes: []SeriesEpisode{{EpisodeNumber: 1, AiredDate: "unknown", AiredDateTimeUTC: "2018-09-01T12:00:00Z"}}},
	}
	for _, tc := range []struct{ season, want int }{{1, 2000}, {3, 2008}, {4, 0}, {5, 2018}, {6, 0}, {0, 0}} {
		if got := SeriesSeasonPremiereYear(seasons, tc.season); got != tc.want {
			t.Errorf("season %d: got %d, want %d", tc.season, got, tc.want)
		}
	}
}
