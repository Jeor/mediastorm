package indexer

import (
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestAudioRankingLanguageCascadeAndCacheInvalidation(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Metadata.Language = []string{"eng", "pol"}
	settings.Playback.PreferredAudioLanguage = "eng"
	profile := &models.UserSettings{}
	profile.Metadata.PrimaryLanguage = "pol"
	svc := &Service{userSettings: staticUserSettingsProvider{settings: profile}}
	opts := SearchOptions{Query: "Moana", UserID: "profile", ClientID: "device"}
	check := func(want string) {
		t.Helper()
		ctx := svc.buildScoringContext(opts, settings, models.FilterSettings{}, models.AnimeFilteringSettings{})
		if ctx.PreferredLang != want {
			t.Fatalf("ranking language=%q, want %q", ctx.PreferredLang, want)
		}
	}
	cacheKey := func() string {
		return svc.searchCacheKey("ranked", opts, settings, nil, models.FilterSettings{}, effectiveFilterBundle{}, models.AnimeFilteringSettings{}, effectiveOverrides{}, nil, effectiveRankingBundle{})
	}
	check("eng") // Metadata=pol does not override global audio=eng.
	before := cacheKey()
	profile.Playback.PreferredAudioLanguage = "pl"
	check("pol")
	if cacheKey() == before {
		t.Fatal("profile audio preference change must invalidate ranked results")
	}
	client := &models.ClientFilterSettings{PreferredAudioLanguage: models.StringPtr("eng")}
	svc.clientSettings = mapClientSettingsProvider{settings: map[string]*models.ClientFilterSettings{"device": client}}
	check("eng")
	before = cacheKey()
	client.PreferredAudioLanguage = models.StringPtr("pol")
	check("pol")
	if cacheKey() == before {
		t.Fatal("device audio preference change must invalidate ranked results")
	}
	client.PreferredAudioLanguage = models.StringPtr("")
	check("") // An explicit device override can clear the preference.
}
