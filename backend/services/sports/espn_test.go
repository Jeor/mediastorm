package sports

import (
	"encoding/json"
	"testing"
)

func TestESPNTeamsToRecordsReturnsCompleteCatalogEntries(t *testing.T) {
	const fixture = `{
		"sports": [{"leagues": [{"teams": [
			{"team": {"id": "1", "displayName": "Atlanta Hawks", "location": "Atlanta", "name": "Hawks", "abbreviation": "ATL", "logos": [{"href": "https://example.test/atl.png"}]}},
			{"team": {"id": "2", "displayName": "Boston Celtics", "location": "Boston", "nickname": "Celtics", "abbreviation": "BOS"}},
			{"team": {"id": "", "displayName": "Invalid"}},
			{"team": {"id": "1", "displayName": "Duplicate Hawks"}}
		]}]}]
	}`

	var payload espnTeamsResponse
	if err := json.Unmarshal([]byte(fixture), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	records := espnTeamsToRecords(payload, League{ID: "nba"})
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].ID != "nba:1" || records[0].Name != "Atlanta Hawks" || records[0].Nickname != "Hawks" {
		t.Fatalf("unexpected first record: %+v", records[0])
	}
	if records[0].LogoURL != "https://example.test/atl.png" {
		t.Fatalf("unexpected logo URL: %q", records[0].LogoURL)
	}
	if records[1].ID != "nba:2" || records[1].Nickname != "Celtics" {
		t.Fatalf("unexpected second record: %+v", records[1])
	}
}

func TestESPNScoreboardMLBSituation(t *testing.T) {
	var raw espnScoreboardSituation
	if err := json.Unmarshal([]byte(`{"balls":0,"strikes":2,"outs":1,"onFirst":true,"onSecond":false,"onThird":false,"batter":{"athlete":{"displayName":"Test Batter"}}}`), &raw); err != nil {
		t.Fatal(err)
	}
	got := scoreboardMLBSituation(&raw, "Top 8th")
	if got.Balls == nil || *got.Balls != 0 || got.Bases != "On 1st" || got.Batter != "Test Batter" || got.Pitcher != "" {
		t.Fatalf("unexpected situation: %+v", got)
	}
	var missing espnScoreboardSituation
	if err := json.Unmarshal([]byte(`{"balls":9,"onFirst":false}`), &missing); err != nil {
		t.Fatal(err)
	}
	got = scoreboardMLBSituation(&missing, "Top 8th")
	if got.Balls != nil || got.Outs != nil || got.Bases != "" {
		t.Fatalf("unknown fields became known: %+v", got)
	}
	if scoreboardMLBSituation(nil, "") != nil {
		t.Fatal("absent situation must stay absent")
	}
}
