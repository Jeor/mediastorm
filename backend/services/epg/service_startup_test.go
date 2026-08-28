package epg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"novastream/models"
)

func TestNewServiceRestoresCachedScheduleAsynchronously(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Now().UTC()
	schedule := models.EPGSchedule{
		Channels: map[string]models.EPGChannel{
			"channel-1": {ID: "channel-1", Name: "Channel One"},
		},
		Programs: map[string][]models.EPGProgram{
			"channel-1": {{Title: "Morning News", Start: now.Add(-time.Hour), Stop: now.Add(time.Hour)}},
		},
		LastUpdated: now,
	}
	data, err := json.Marshal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	epgDir := filepath.Join(cacheDir, "epg")
	if err := os.MkdirAll(epgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epgDir, epgCacheFile), data, 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService(cacheDir, nil)
	<-service.restoreDone

	channels := service.GetAllChannels()
	if len(channels) != 1 || channels["channel-1"].Name != "Channel One" {
		t.Fatalf("restored channels = %+v", channels)
	}
	programs := service.GetSchedule("channel-1", now.Add(-2*time.Hour), now.Add(2*time.Hour))
	if len(programs) != 1 || programs[0].Title != "Morning News" {
		t.Fatalf("restored programs = %+v", programs)
	}
}

func TestCachedRestoreDoesNotReplaceNewerSchedule(t *testing.T) {
	initial := &models.EPGSchedule{
		Channels: make(map[string]models.EPGChannel),
		Programs: make(map[string][]models.EPGProgram),
	}
	newer := &models.EPGSchedule{
		Channels: map[string]models.EPGChannel{"new": {ID: "new"}},
		Programs: make(map[string][]models.EPGProgram),
	}
	service := &Service{
		schedule: newer,
	}
	restored := &models.EPGSchedule{
		Channels: map[string]models.EPGChannel{"old": {ID: "old"}},
		Programs: make(map[string][]models.EPGProgram),
	}

	if service.installRestoredSchedule(initial, restored) {
		t.Fatal("stale cached schedule was installed")
	}
	if service.schedule != newer {
		t.Fatal("newer schedule was replaced")
	}
}
