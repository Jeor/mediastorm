package filter

import (
	"strings"
	"testing"

	"novastream/models"
)

func TestSkipNameFilteringKeepsOtherFilters(t *testing.T) {
	movie := Options{ExpectedTitle: "The Matrix", ExpectedYear: 1999, IsMovie: true}
	series := Options{ExpectedTitle: "Breaking Bad", ExpectedYear: 2008, TargetSeason: 1, TargetEpisode: 1}
	daily := series
	daily.IsDaily = true
	daily.TargetAirDate = "2026-10-02"
	for _, tc := range []struct {
		name, title string
		opts        Options
		service     models.ContentServiceType
		bypass      bool
		attrs       map[string]string
		size        int64
		wantPass    bool
		reason      string
	}{
		{name: "numeric movie default off", title: "4673019.mp4", opts: movie, service: models.ServiceTypeDebrid, reason: "title similarity"},
		{name: "numeric movie opt in", title: "4673019.mp4", opts: movie, service: models.ServiceTypeDebrid, bypass: true, wantPass: true},
		{name: "numeric episode", title: "986606.mp4", opts: series, service: models.ServiceTypeDebrid, bypass: true, wantPass: true},
		{name: "mismatched title year and episode", title: "Other.Show.1990.S02E03.1080p.mkv", opts: series, service: models.ServiceTypeDebrid, bypass: true, wantPass: true},
		{name: "daily filename date", title: "Other.Show.2026.10.01.mp4", opts: daily, service: models.ServiceTypeDebrid, bypass: true, wantPass: true},
		{name: "unrelated usenet unaffected", title: "986606.mp4", opts: movie, service: models.ServiceTypeUsenet, bypass: true, reason: "title similarity"},
		{name: "unknown service unaffected", title: "986606.mp4", opts: movie, bypass: true, reason: "title similarity"},
		{name: "movie size limit", title: "986606.mp4", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, MaxSizeMovieGB: 1}, service: models.ServiceTypeDebrid, bypass: true, size: 2 * 1024 * 1024 * 1024, reason: "size"},
		{name: "episode size limit", title: "986606.mp4", opts: Options{ExpectedTitle: "Breaking Bad", TargetSeason: 1, TargetEpisode: 1, MaxSizeEpisodeGB: 1}, service: models.ServiceTypeDebrid, bypass: true, size: 2 * 1024 * 1024 * 1024, reason: "size"},
		{name: "structured resolution", title: "986606.mp4", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, MaxResolution: "720p"}, service: models.ServiceTypeDebrid, bypass: true, attrs: map[string]string{"resolution": "1080p"}, reason: "resolution"},
		{name: "parsed resolution", title: "Other.Movie.2160p.mkv", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, MaxResolution: "720p"}, service: models.ServiceTypeDebrid, bypass: true, reason: "resolution"},
		{name: "structured HDR", title: "986606.mp4", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, HDRDVPolicy: HDRDVPolicyNoExclusion}, service: models.ServiceTypeDebrid, bypass: true, attrs: map[string]string{"hdr": "HDR10"}, reason: "HDR"},
		{name: "parsed HDR", title: "Other.Movie.2160p.HDR.mkv", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, HDRDVPolicy: HDRDVPolicyNoExclusion}, service: models.ServiceTypeDebrid, bypass: true, reason: "HDR"},
		{name: "required terms", title: "986606.mp4", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, RequiredTerms: []string{"remux"}}, service: models.ServiceTypeDebrid, bypass: true, reason: "required terms"},
		{name: "excluded terms", title: "986606.mp4", opts: Options{ExpectedTitle: "The Matrix", IsMovie: true, FilterOutTerms: []string{"986606"}}, service: models.ServiceTypeDebrid, bypass: true, reason: "filter-out term"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrs := map[string]string{}
			for k, v := range tc.attrs {
				attrs[k] = v
			}
			if tc.bypass {
				attrs[SkipNameFilteringAttribute] = "true"
			}
			result := models.NZBResult{Title: tc.title, ServiceType: tc.service, SizeBytes: tc.size, Attributes: attrs}
			details := ResultsWithDetails([]models.NZBResult{result}, tc.opts)
			if len(details) != 1 || details[0].Passed != tc.wantPass {
				t.Fatalf("details = %+v", details)
			}
			if !tc.wantPass && !strings.Contains(details[0].RejectReason, tc.reason) {
				t.Fatalf("reason = %q, want %q", details[0].RejectReason, tc.reason)
			}
			if tc.bypass && tc.wantPass && details[0].Result.Attributes["titleMatch"] != "" {
				t.Fatal("bypassed name was marked verified")
			}
		})
	}
}
