package handlers

import (
	"context"
	"testing"

	"novastream/models"
)

type seasonYearMetadataProvider struct{}

func (seasonYearMetadataProvider) SeriesDetails(context.Context, models.SeriesDetailsQuery) (*models.SeriesDetails, error) {
	return &models.SeriesDetails{
		Title: models.Title{Name: "Example Show", Year: 2000},
		Seasons: []models.SeriesSeason{
			{Number: 1, Episodes: []models.SeriesEpisode{{SeasonNumber: 1, EpisodeNumber: 1, AiredDate: "2000-01-01"}}},
			{Number: 3, Episodes: []models.SeriesEpisode{
				{SeasonNumber: 3, EpisodeNumber: 1, AiredDate: "2008-01-01"},
				{SeasonNumber: 3, EpisodeNumber: 8, AiredDate: "2012-01-01"},
			}},
		},
	}, nil
}

func TestSearchAndPrequeueUseRequestedSeasonYear(t *testing.T) {
	search := &IndexerHandler{MetadataSvc: seasonYearMetadataProvider{}}
	prequeue := &PrequeueHandler{metadataSvc: seasonYearMetadataProvider{}}
	for _, tc := range []struct {
		query                                    string
		season, episode, seasonYear, episodeYear int
	}{
		{"Example Show S03E08", 3, 8, 2008, 2012},
		{"Example Show S01E01", 1, 1, 2000, 2000},
	} {
		s := search.getSeriesSearchMetadata(t.Context(), tc.query, 2000, "")
		p := prequeue.createEpisodeResolverAndLookupAbsoluteEp(t.Context(), "", "Example Show", 2000, "", &models.EpisodeReference{SeasonNumber: tc.season, EpisodeNumber: tc.episode})
		if s == nil || s.SeasonPremiereYear != tc.seasonYear || s.EpisodeAirYear != tc.episodeYear {
			t.Fatalf("search %s: %+v", tc.query, s)
		}
		if p == nil || p.SeasonPremiereYear != tc.seasonYear || p.EpisodeAirYear != tc.episodeYear {
			t.Fatalf("prequeue %s: %+v", tc.query, p)
		}
	}
}
