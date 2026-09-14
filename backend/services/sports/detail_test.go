package sports

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"novastream/models"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type detailTransport func(*http.Request) (*http.Response, error)

func (f detailTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func detailFixture(t *testing.T) (models.SportsGame, mlbSummary, []byte) {
	t.Helper()
	raw, e := os.ReadFile("testdata/mlb-summary-final.json")
	if e != nil {
		t.Fatal(e)
	}
	var p mlbSummary
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	g := models.SportsGame{ID: "401816843", League: "mlb", AwayTeam: models.SportsTeam{ID: "15"}, HomeTeam: models.SportsTeam{ID: "22"}}
	return g, p, raw
}
func TestMLBDetailVerifiedFinal(t *testing.T) {
	g, p, _ := detailFixture(t)
	got, e := normalizeMLBDetail(g, p, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if got.HomeTeam.Score != "1" || got.AwayTeam.Score != "0" || got.Status != models.SportsGameFinal {
		t.Fatalf("wrong snapshot: %+v", got)
	}
	d := got.Detail
	if d.Comparisons[2].Label != "Errors" || d.Comparisons[2].Home != "0" {
		t.Fatal("missing zero errors")
	}
	if len(d.Periods) != 9 || d.Periods[8].Home != "–" {
		t.Fatal("unplayed inning must not become zero")
	}
	if !d.Capabilities.Plays || !d.Capabilities.Stats || d.Plays[0].ID != p.Plays[len(p.Plays)-1].ID {
		t.Fatal("missing capabilities or wrong timeline order")
	}
}
func TestMLBDetailRejectsWrongIdentity(t *testing.T) {
	g, p, _ := detailFixture(t)
	p.Header.Competitions[0].Competitors[0].ID = "wrong"
	if _, e := normalizeMLBDetail(g, p, time.Now()); e == nil {
		t.Fatal("accepted mismatched team")
	}
}
func TestMLBDetailPregameDoesNotExposeSeasonStats(t *testing.T) {
	g, p, _ := detailFixture(t)
	p.Header.Competitions[0].Status.Type.State = "pre"
	got, e := normalizeMLBDetail(g, p, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if got.Detail.Capabilities.Stats || got.Detail.Capabilities.Plays || len(got.Detail.Periods) > 0 {
		t.Fatal("pregame leaked played statistics")
	}
}
func TestMLBDetailCacheAndFailure(t *testing.T) {
	g, _, raw := detailFixture(t)
	calls := 0
	fail := false
	s := NewService(t.TempDir())
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if fail {
			return nil, errors.New("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	first := s.EnrichGame(context.Background(), g)
	s.EnrichGame(context.Background(), g)
	if calls != 1 {
		t.Fatal("cache missed")
	}
	entry := s.details[g.ID]
	entry.expires = time.Time{}
	s.details[g.ID] = entry
	fail = true
	stale := s.EnrichGame(context.Background(), g)
	if stale.Detail == nil || !stale.Detail.Stale || !stale.Detail.UpdatedAt.Equal(first.Detail.UpdatedAt) {
		t.Fatal("lost last good snapshot")
	}
	// Repeated failures preserve the original timestamp; recovery clears stale.
	entry = s.details[g.ID]
	entry.expires = time.Time{}
	s.details[g.ID] = entry
	stale = s.EnrichGame(context.Background(), g)
	if !stale.Detail.Stale || !stale.Detail.UpdatedAt.Equal(first.Detail.UpdatedAt) {
		t.Fatal("repeated outage changed last good timestamp")
	}
	entry = s.details[g.ID]
	entry.expires = time.Time{}
	s.details[g.ID] = entry
	fail = false
	recovered := s.EnrichGame(context.Background(), g)
	if recovered.Detail == nil || recovered.Detail.Stale || !recovered.Detail.UpdatedAt.After(first.Detail.UpdatedAt) {
		t.Fatal("successful retry did not recover fresh detail")
	}
	fail = true
	s.details = map[string]detailCacheEntry{}
	s.EnrichGame(context.Background(), g)
	entry = s.details[g.ID]
	entry.expires = time.Time{}
	s.details[g.ID] = entry
	s.EnrichGame(context.Background(), g)
}

func TestMLBLiveSituationNamesAndZeroCounts(t *testing.T) {
	raw, err := os.ReadFile("testdata/mlb-summary-live.json")
	if err != nil {
		t.Fatal(err)
	}
	var p mlbSummary
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{ID: "401816854", League: "mlb", AwayTeam: models.SportsTeam{ID: "5"}, HomeTeam: models.SportsTeam{ID: "1"}}
	got, err := normalizeMLBDetail(game, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := got.Detail.Sport
	if s == nil || s.Balls == nil || *s.Balls != 0 || s.Outs == nil || *s.Outs != 0 || s.Batter != "Steven Kwan" || s.Pitcher != "Brandon Young" {
		t.Fatalf("wrong live situation %+v", s)
	}
	p.Situation.Outs = nil
	p.Situation.Batter.PlayerID = json.RawMessage(`99999`)
	got, err = normalizeMLBDetail(game, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Detail.Sport.Outs != nil || got.Detail.Sport.Batter != "" {
		t.Fatal("invented missing situation")
	}
	p.Header.Competitions[0].Status.Type.State = "post"
	got, _ = normalizeMLBDetail(game, p, time.Now())
	if got.Detail.Sport != nil {
		t.Fatal("final retained live situation")
	}
}

func TestMLBOccupiedBaseFromLivePayload(t *testing.T) {
	raw, err := os.ReadFile("testdata/mlb-summary-live.json")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["situation"] = fields["situationOccupiedSample"]
	raw, _ = json.Marshal(fields)
	var p mlbSummary
	json.Unmarshal(raw, &p)
	g := models.SportsGame{ID: "401816854", League: "mlb", AwayTeam: models.SportsTeam{ID: "5"}, HomeTeam: models.SportsTeam{ID: "1"}}
	got, err := normalizeMLBDetail(g, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Detail.Sport.Bases != "Runner on 2nd" {
		t.Fatalf("wrong bases: %+v", got.Detail.Sport)
	}
	p.Situation.OnSecond = nil
	got, _ = normalizeMLBDetail(g, p, time.Now())
	if got.Detail.Sport.Bases != "" {
		t.Fatal("invented empty-base declaration")
	}
}

func TestMLBDetailFailureCacheRemainsBounded(t *testing.T) {
	s := NewService(t.TempDir())
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	for i := 0; i < 80; i++ {
		s.EnrichGame(context.Background(), models.SportsGame{ID: strconv.Itoa(i), League: "mlb"})
	}
	if len(s.details) > 64 {
		t.Fatalf("outage cache grew to %d entries", len(s.details))
	}
	if _, ok := s.details["79"]; !ok {
		t.Fatal("latest failure missing retry backoff")
	}
}

func TestMLBLivePitchCountUsesCurrentPitcherAndNamedColumn(t *testing.T) {
	for _, tc := range []struct {
		name, state, pitcherID, athleteID, teamID string
		names, stats                              []string
		want                                      string
	}{
		{"reordered", "in", "42", "42", "15", []string{"PC", "IP", "PC-ST"}, []string{"62", "4.1", "62-42"}, "62"},
		{"zero", "in", "42", "42", "15", []string{"IP", "PC"}, []string{"0.0", "0"}, "0"},
		{"different pitcher", "in", "42", "43", "15", []string{"PC"}, []string{"62"}, ""},
		{"different team", "in", "42", "42", "99", []string{"PC"}, []string{"62"}, ""},
		{"missing pitcher", "in", "", "", "15", []string{"PC"}, []string{"62"}, ""},
		{"missing column", "in", "42", "42", "15", []string{"PC-ST"}, []string{"62-42"}, ""},
		{"missing value", "in", "42", "42", "15", []string{"IP", "PC"}, []string{"4.1"}, ""},
		{"invalid value", "in", "42", "42", "15", []string{"PC"}, []string{"--"}, ""},
		{"negative value", "in", "42", "42", "15", []string{"PC"}, []string{"-1"}, ""},
		{"fractional value", "in", "42", "42", "15", []string{"PC"}, []string{"1.5"}, ""},
		{"final", "post", "42", "42", "15", []string{"PC"}, []string{"62"}, ""},
		{"pregame", "pre", "42", "42", "15", []string{"PC"}, []string{"62"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			game, payload, _ := detailFixture(t)
			payload.Header.Competitions[0].Status.Type.State = tc.state
			// Match the verified ESPN boxscore shape, including an unrelated pitcher first.
			raw, err := json.Marshal(map[string]any{
				"situation": map[string]any{"pitcher": map[string]string{"playerId": tc.pitcherID}},
				"boxscore": map[string]any{"players": []any{map[string]any{
					"team": map[string]string{"id": tc.teamID},
					"statistics": []any{map[string]any{
						"names": tc.names,
						"athletes": []any{
							map[string]any{"athlete": map[string]string{"id": "other"}, "stats": []string{"99", "99", "99"}},
							map[string]any{"athlete": map[string]string{"id": tc.athleteID}, "stats": tc.stats},
						},
					}},
				}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			got, err := normalizeMLBDetail(game, payload, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if tc.state != "in" {
				if got.Detail.Sport != nil {
					t.Fatal("non-live game exposed current pitcher situation")
				}
				return
			}
			if got.Detail.Sport == nil {
				t.Fatal("missing live situation")
			}
			encoded, err := json.Marshal(got.Detail.Sport)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if value := string(fields["pitcherPitchCount"]); value != tc.want {
				t.Fatalf("pitcherPitchCount = %q, want %q", value, tc.want)
			}
		})
	}
}
