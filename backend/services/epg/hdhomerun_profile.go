package epg

import (
	"strings"

	"novastream/config"
	"novastream/models"
)

// ProfileHDHomeRunSources returns explicit tuner overrides; inherited global
// tuners are already collected from settings. Pointer fields preserve opt-outs.
func ProfileHDHomeRunSources(profile models.LiveTVSettings, global config.LiveSettings) []config.LivePlaylistSource {
	enabled := global.EPG.Enabled
	if profile.EPG != nil && profile.EPG.Enabled != nil {
		enabled = *profile.EPG.Enabled
	}
	sources := profile.Sources
	if len(sources) == 0 {
		sources = profile.PlaylistSources
	}
	var result []config.LivePlaylistSource
	for _, source := range sources {
		if !strings.EqualFold(strings.TrimSpace(source.Mode), "hdhomerun") {
			continue
		}
		guide := config.EPGSettings{Enabled: enabled}
		if source.EPG != nil {
			if source.EPG.Enabled != nil {
				guide.Enabled = *source.EPG.Enabled
			}
			if source.EPG.XmltvUrl != nil {
				guide.XmltvUrl = *source.EPG.XmltvUrl
			}
		}
		result = append(result, config.LivePlaylistSource{ID: source.ID, Name: source.Name, Mode: "hdhomerun", HDHomeRunHost: source.HDHomeRunHost, Enabled: source.Enabled, EPG: guide})
	}
	if len(sources) == 0 && profile.Mode != nil && strings.EqualFold(*profile.Mode, "hdhomerun") {
		address := global.HDHomeRunHost
		if profile.HDHomeRunHost != nil {
			address = *profile.HDHomeRunHost
		}
		guide := config.EPGSettings{Enabled: enabled}
		if profile.EPG != nil && profile.EPG.XmltvUrl != nil {
			guide.XmltvUrl = *profile.EPG.XmltvUrl
		}
		result = append(result, config.LivePlaylistSource{Mode: "hdhomerun", HDHomeRunHost: address, EPG: guide})
	}
	return result
}
