package sports

import (
	"context"
	"fmt"
	"testing"

	"novastream/models"
)

func TestRefreshRetainsCachedGamesWhenContextIsCanceled(t *testing.T) {
	service := NewService(t.TempDir())
	leagues := make([]League, 64)
	games := make(map[string][]models.SportsGame, len(leagues))
	for i := range leagues {
		id := fmt.Sprintf("cached-%d", i)
		leagues[i] = League{ID: id, Sport: "football", Slug: "nfl", EventKind: "matchup"}
		games[id] = []models.SportsGame{{ID: "game-" + id, League: id}}
	}
	service.leagues = leagues
	service.games = games

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Refresh(ctx); err == nil {
		t.Fatal("expected canceled refresh to return an error")
	}

	for _, league := range leagues {
		cached := service.GetScoreboard(league.ID)
		if len(cached) != 1 || cached[0].ID != "game-"+league.ID {
			t.Fatalf("cached games for %s were lost: %+v", league.ID, cached)
		}
	}
}
