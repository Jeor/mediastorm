package filter

import (
	"testing"

	"novastream/models"
)

func TestSeasonYearCannotAdmitAnotherSeasonOrMovie(t *testing.T) {
	for _, tc := range []struct {
		title  string
		movie  bool
		season int
	}{
		{"Example.Show.2008.S01E01.1080p.WEB", false, 3},
		{"Example.Show.2008.1080p.WEB", true, 3},
		{"Example.Show.2008.S01E01.1080p.WEB", false, 1},
	} {
		year := 2008
		if tc.season == 1 {
			year = 2000
		}
		results := Results([]models.NZBResult{{Title: tc.title}}, Options{ExpectedTitle: "Example Show", ExpectedYear: 2000, SeasonPremiereYear: year, IsMovie: tc.movie, TargetSeason: tc.season, TargetEpisode: 1})
		if len(results) != 0 {
			t.Errorf("unrelated release accepted: %s", tc.title)
		}
	}
}
