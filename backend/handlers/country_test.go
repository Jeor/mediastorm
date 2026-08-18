package handlers

import (
	"context"
	"testing"

	"novastream/models"
)

type countrySeriesDetailsProvider struct{}

func (countrySeriesDetailsProvider) SeriesDetails(context.Context, models.SeriesDetailsQuery) (*models.SeriesDetails, error) {
	return &models.SeriesDetails{
		Title:   models.Title{Name: "Shameless", Year: 2004, CountryCode: "gbr"},
		Seasons: []models.SeriesSeason{{Number: 1, EpisodeCount: 7}},
	}, nil
}

func TestSeriesSearchMetadataPropagatesCountryCode(t *testing.T) {
	handler := &IndexerHandler{MetadataSvc: countrySeriesDetailsProvider{}}
	metadata := handler.getSeriesSearchMetadata(context.Background(), "Shameless S01E01", 2004, "")
	if metadata == nil || metadata.CountryCode != "gbr" {
		t.Fatalf("unexpected series search metadata: %+v", metadata)
	}
}

func TestPrequeueSeriesMetadataPropagatesCountryCode(t *testing.T) {
	handler := &PrequeueHandler{metadataSvc: countrySeriesDetailsProvider{}}
	metadata := handler.createEpisodeResolverAndLookupAbsoluteEp(context.Background(), "", "Shameless", 2004, "", nil)
	if metadata == nil || metadata.CountryCode != "gbr" {
		t.Fatalf("unexpected prequeue series metadata: %+v", metadata)
	}
}
