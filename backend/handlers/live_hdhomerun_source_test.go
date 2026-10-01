package handlers

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"novastream/config"
	"novastream/models"
)

type hdHomeRunProfilePreferences struct{ profile *models.UserSettings }

func (p hdHomeRunProfilePreferences) Get(string) (*models.UserSettings, error) { return p.profile, nil }
func (p hdHomeRunProfilePreferences) GetUsersWithOverrides() map[string]bool {
	return map[string]bool{"profile": true}
}

func TestHDHomeRunProfileLineupUsesConfiguredPrivateOrigin(t *testing.T) {
	settings := config.DefaultSettings()
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	preferences := hdHomeRunProfilePreferences{profile: &models.UserSettings{LiveTV: models.LiveTVSettings{Sources: []models.LivePlaylistSource{{ID: "profile-hdhr", Mode: "hdhomerun", HDHomeRunHost: "192.168.1.200"}}}}}
	handler := &LiveHandler{cfgManager: manager, userSettingsSvc: preferences}
	if _, err := handler.parseRemoteURL(context.Background(), "http://192.168.1.200/lineup.m3u"); err != nil {
		t.Fatalf("configured profile tuner lineup rejected: %v", err)
	}
	if _, err := handler.parseRemoteURL(context.Background(), "http://192.168.1.201/lineup.m3u"); err == nil {
		t.Fatal("unconfigured tuner origin allowed")
	}
	if _, err := handler.parseRemoteURL(context.Background(), "http://192.168.1.200:5004/auto/v5.1"); err == nil {
		t.Fatal("tuner stream port authorized as generic media")
	}
}

func TestNativeHDHomeRunSourceResolutionAndStreamAuthorization(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Live.Sources = []config.LivePlaylistSource{{ID: "hdhr", Mode: "hdhomerun", HDHomeRunHost: "192.168.1.100"}}
	sources := resolvedLiveSources(buildGlobalLiveSource(settings))
	if len(sources) != 1 || sources[0].PlaylistURL != "http://192.168.1.100/lineup.m3u" || sources[0].Mode != "m3u" {
		t.Fatalf("incorrect native source resolution: %+v", sources)
	}
	stream := "http://192.168.1.100:5004/auto/v5.1"
	channels := tagChannelsWithSource(parseM3UPlaylist("#EXTM3U\n#EXTINF:-1 group-title=\"Favorites\",5.1 TESTTV\n"+stream+"\n"), sources[0], true)
	if len(channels) != 1 || channels[0].TvgID != "TESTTV" || channels[0].Group != "Favorites" {
		t.Fatalf("missing guide match or favorite group: %+v", channels)
	}
	catalog := staticLiveChannelProvider{channels: channels}
	request := httptest.NewRequest("GET", "/api/live/stream?sourceId=hdhr", nil)
	allowed, err := authorizeLiveStreamURL(request, stream, staticSecurityConfigProvider{settings}, catalog)
	if !allowed || err != nil {
		t.Fatalf("native tuner playback rejected: %v", err)
	}
	if allowed, err := authorizeLiveStreamURL(request, "http://192.168.1.100:5004/auto/v9.9", staticSecurityConfigProvider{settings}, catalog); allowed || err == nil {
		t.Fatal("unlisted tuner stream authorized")
	}
	policy := configuredProviderHostPolicy(staticSecurityConfigProvider{settings})
	if !policy("192.168.1.100", "80") || policy("192.168.1.100", "5004") {
		t.Fatal("tuner policy widened beyond configured HTTP origin")
	}
	// Legacy global configuration and profile tuner addresses use the same lineup path.
	global := models.ResolvedLiveSource{Mode: "hdhomerun", HDHomeRunHost: "hdhomerun.local"}
	if sources := resolvedLiveSources(global); len(sources) != 1 || sources[0].PlaylistURL != "http://hdhomerun.local/lineup.m3u" {
		t.Fatal("legacy tuner source not resolved")
	}
	address := "192.168.1.200"
	resolved := models.ResolveLiveSource(&models.LiveTVSettings{HDHomeRunHost: &address}, &global)
	if sources := resolvedLiveSources(resolved); len(sources) != 1 || sources[0].PlaylistURL != "http://192.168.1.200/lineup.m3u" {
		t.Fatal("profile tuner override not resolved")
	}
}

func TestHDHomeRunGuideTaskCreatedAndAddressChangesTriggerRefresh(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Live.Sources = []config.LivePlaylistSource{{ID: "hdhr", Mode: "hdhomerun", HDHomeRunHost: "192.168.1.100", EPG: config.EPGSettings{Enabled: true}}}
	h := &SettingsHandler{}
	if !h.EnsureEPGTaskForGuide(&settings, "test") || len(settings.ScheduledTasks.Tasks) != 1 || !settings.ScheduledTasks.Tasks[0].Enabled {
		t.Fatal("automatic HDHomeRun guide did not create enabled EPG task")
	}
	h.ensurePlaylistTaskIfConfigured(&settings)
	if len(settings.ScheduledTasks.Tasks) != 2 {
		t.Fatal("native tuner did not create playlist refresh task")
	}
	fingerprint := epgConfigFingerprint(settings)
	settings.Live.Sources[0].HDHomeRunHost = "192.168.1.200"
	if fingerprint == epgConfigFingerprint(settings) {
		t.Fatal("tuner address change does not trigger guide refresh")
	}
}
