package sports

import (
	"novastream/models"
	"testing"
	"time"
)

func TestCircuitExactEventAssociation(t *testing.T) {
	event := models.SportsEvent{League: "f1", ProviderEventID: "600057442", Title: "Pirelli Italian Grand Prix", StartTime: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)}
	attachRaceCircuit(&event)
	if event.Circuit == nil || event.Circuit.ID != "it-1922" || len(event.Circuit.Coordinates) < 3 {
		t.Fatal("verified Monza map missing")
	}
	for _, changed := range []models.SportsEvent{
		{League: "f1", ProviderEventID: "600060990", Title: "Gulf Air Bahrain Grand Prix in Malaysia", StartTime: event.StartTime},
		{League: "f1", ProviderEventID: event.ProviderEventID, Title: "Changed venue", StartTime: event.StartTime},
		{League: "f1", ProviderEventID: event.ProviderEventID, Title: event.Title, StartTime: event.StartTime.AddDate(1, 0, 0)},
	} {
		attachRaceCircuit(&changed)
		if changed.Circuit != nil {
			t.Fatal("unverified event received a map")
		}
	}
	if len(referenceCircuits) != 24 {
		t.Fatal("missing licensed geometry", len(referenceCircuits))
	}
}
