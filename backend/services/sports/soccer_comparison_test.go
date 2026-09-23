package sports

import (
	"encoding/json"
	"novastream/models"
	"testing"
	"time"
)

func TestSoccerExpandedComparisons(t *testing.T) {
	raw := `{"header":{"id":"game","competitions":[{"status":{"type":{"state":"in"}},"competitors":[{"id":"a","homeAway":"away"},{"id":"h","homeAway":"home"}]}]},"boxscore":{"teams":[{"team":{"id":"a"},"statistics":[{"name":"totalPasses","displayValue":"123"},{"name":"passPct","displayValue":"81.2"},{"name":"interceptions","displayValue":"0"}]},{"team":{"id":"h"},"statistics":[{"name":"totalPasses","displayValue":"145"},{"name":"passPct","displayValue":"84.5"},{"name":"interceptions","displayValue":"2"}]}]}}`
	var p teamSportSummary
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	g, err := normalizeTeamDetail(models.SportsGame{ID: "game", League: "soccer-eng.1", AwayTeam: models.SportsTeam{ID: "a"}, HomeTeam: models.SportsTeam{ID: "h"}}, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Detail.Comparisons) != 3 {
		t.Fatalf("expected supplied statistics only: %+v", g.Detail.Comparisons)
	}
	if g.Detail.Comparisons[0].Label != "Passes" || g.Detail.Comparisons[0].Away != "123" {
		t.Fatal(g.Detail.Comparisons)
	}
	if g.Detail.Comparisons[2].Away != "0" {
		t.Fatal("zero interceptions lost")
	}
}
