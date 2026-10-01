package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"novastream/config"
	"novastream/services/epg"
)

func TestHDHomeRunDeadlineOverridesOrdinaryGuideFrequency(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Live.Sources = []config.LivePlaylistSource{{Mode: "hdhomerun", HDHomeRunHost: "192.168.1.100", EPG: config.EPGSettings{Enabled: true}}}
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	s := NewService(manager, nil, nil, nil)
	s.SetEPGService(epg.NewService(t.TempDir(), manager))
	now := time.Now()
	task := config.ScheduledTask{ID: "guide", Type: config.ScheduledTaskTypeEPGRefresh, Frequency: config.ScheduledTaskFrequency12Hours, LastRunAt: &now}
	if !s.shouldRun(task) {
		t.Fatal("due tuner guide waits for ordinary 12-hour frequency")
	}
	task.Frequency = config.ScheduledTaskFrequencyOnce
	if s.shouldRun(task) {
		t.Fatal("one-shot guide task unexpectedly repeated")
	}
	task.Frequency = config.ScheduledTaskFrequency12Hours
	s.taskRunning[task.ID] = true
	if s.shouldRun(task) {
		t.Fatal("already-running guide task started again")
	}
}
