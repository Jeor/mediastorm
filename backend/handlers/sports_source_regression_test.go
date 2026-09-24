package handlers

import (
	"novastream/models"
	"testing"
)

func TestLiveCatalogMatchingRegressions(t *testing.T) {
	game := models.SportsGame{League: "atp", HomeTeam: models.SportsTeam{Name: "TBD"}, AwayTeam: models.SportsTeam{Name: "TBD"}}
	if got := matchGameToChannels(game, []LiveChannel{{ID: "tbd", Name: "US: Roar (TBD)", URL: "https://example.com"}}, nil, ""); len(got) != 0 {
		t.Fatal("placeholder participants matched a channel", got)
	}
	game.HomeTeam = models.SportsTeam{Name: "Denis Shapovalov", Nickname: "Shapovalov"}
	game.AwayTeam = models.SportsTeam{Name: "Vit Kopriva", Nickname: "Kopriva"}
	for _, name := range []string{"⏪ Denis Shapovalov - Vit Kopriva", "Tallon Griekspoor - Denis Shapovalov"} {
		if got := selectableSportsMatches(matchGameToChannels(game, []LiveChannel{{ID: "tennis", Name: name, URL: "https://example.com"}}, nil, "")); len(got) != 0 {
			t.Fatalf("wrong/replayed tennis match %s: %+v", name, got)
		}
	}
	if got := selectableSportsMatches(matchGameToChannels(game, []LiveChannel{{ID: "tennis", Name: "Denis Shapovalov - Vit Kopriva", URL: "https://example.com"}}, nil, "")); len(got) != 1 {
		t.Fatal("lost exact tennis opponents", got)
	}
	cycle := models.SportsGame{Title: "Tour de France", League: "cycling", EventKind: "cycling-stage", EventContext: "Stage 3"}
	for _, name := range []string{"DP WORLD TOUR: Open de France (France)", "LIVE: DP World Tour FedEx Open de France"} {
		if e := scoreWatchEvent(name, cycle); e.score != 0 {
			t.Fatal("golf matched cycling", name, e)
		}
	}
	cycle.Title = "CRO Race"
	if e := scoreWatchEvent("🔴 LIVE: Cro Race Stage 3 | Men | Krk – Labin (177.1km)", cycle); e.score < 0.65 {
		t.Fatal("lost cycling source without sport label", e)
	}
	if e := scoreWatchEvent("CRO Race Stage 4", cycle); e.score != 0 {
		t.Fatal("accepted different stage", e)
	}
}
