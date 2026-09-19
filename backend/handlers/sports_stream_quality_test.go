package handlers

import (
	"encoding/json"
	"novastream/models"
	"slices"
	"testing"
)

func TestReportedSportsQuality(t *testing.T) {
	for _, tc := range []struct {
		label   string
		height  int
		bitrate int64
	}{
		{"Game 2160p 12 Mbps", 2160, 12000000}, {"Game 4K", 2160, 0}, {"Game FHD", 1080, 0}, {"Game 720p", 720, 0},
		{"Game HD", 0, 0}, {"Game 1080 fans", 0, 0}, {"Game 4K 720p", 720, 0}, {"Team UHD 50fps", 2160, 0},
	} {
		q := reportedSportsQuality(tc.label)
		if tc.height == 0 && tc.bitrate == 0 {
			if q != nil {
				t.Errorf("%q should be unknown: %+v", tc.label, q)
			}
			continue
		}
		if q == nil || q.ResolutionHeight != tc.height || q.BitrateBps != tc.bitrate || q.Origin != "label" {
			t.Errorf("%q: %+v", tc.label, q)
		}
	}
}
func TestSportsQualityRankingPreservesConfidence(t *testing.T) {
	matches := []models.SportsStreamMatch{
		{ChannelName: "Possible 4K", Confidence: .74},
		{ChannelName: "Z 1080p", Confidence: .9},
		{ChannelName: "A 720p", Confidence: .9},
	}
	sortSportsStreamMatches(matches)
	if matches[0].ChannelName != "Z 1080p" || matches[1].ChannelName != "A 720p" || matches[2].ChannelName != "Possible 4K" {
		t.Fatalf("wrong ranking: %+v", matches)
	}
}
func TestStremioQualityKeepsOriginalIndexes(t *testing.T) {
	got := playableStremioStreamOptions([]stremioStream{{URL: "https://example.test/a", Name: "720p"}, {URL: "magnet:unsupported", Name: "4K"}, {URL: "https://example.test/b", Description: "4K"}})
	if len(got) != 2 || got[0].Index != 2 || got[1].Index != 0 || got[0].ReportedQuality.ResolutionHeight != 2160 {
		t.Fatalf("wrong options %+v", got)
	}
}

func TestStremioPlaybackUsesQualityOrderAndPreservesSelection(t *testing.T) {
	var response stremioStreamResponse
	err := json.Unmarshal([]byte(`{"streams":[
 {"url":"https://example.test/hd","title":"Quality: HD","resolution":"HD","bitrate":null},
 {"url":"https://example.test/fullhd","resolution":"1920x1080","quality":"1080p","bitrate":"8.0 Mbps"},
 {"url":"magnet:unsupported","quality":"4K"},
 {"url":"https://example.test/slow","resolution":1080,"bitrate":4000000},
 {"url":"https://example.test/unknown","resolution":{},"bitrate":[]}
 ]}`), &response)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := playableStremioStream(response.Streams, -1)
	if !ok || got.Index != 1 || !slices.Equal(got.AvailableIndexes, []int{1, 3, 0, 4}) {
		t.Fatalf("wrong automatic selection: %+v", got)
	}
	options := playableStremioStreamOptions(response.Streams)
	if options[0].Index != 1 || options[0].ReportedQuality.BitrateBps != 8000000 {
		t.Fatalf("picker disagrees with playback: %+v", options)
	}
	chosen, ok := playableStremioStream(response.Streams, 0)
	if !ok || chosen.Index != 0 {
		t.Fatalf("overrode explicit choice: %+v", chosen)
	}
	if _, ok := playableStremioStream(response.Streams, 2); ok {
		t.Fatal("selected unsupported stream")
	}
	if _, ok := playableStremioStream(response.Streams, 99); ok {
		t.Fatal("silently replaced missing selection")
	}
}

func TestStremioQualityMetadataFallbackAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		payload string
		height  int
		bitrate int64
	}{
		{`{"resolution":"1920x1080","quality":"720p","title":"4K","bitrate":"8 Mbps"}`, 720, 8000000},
		{`{"resolution":null,"quality":{},"title":"1080p 6 Mbps","bitrate":false}`, 1080, 6000000},
		{`{"resolution":"HD","bitrate":"8000 kbps"}`, 0, 8000000},
		{`{"resolution":"1920x1080","bitrate":8000000}`, 1080, 8000000},
		{`{"resolution":-1080,"bitrate":-100}`, 0, 0},
		{`{"resolution":"1080","bitrate":"8000000"}`, 1080, 8000000},
	} {
		var s stremioStream
		if err := json.Unmarshal([]byte(tc.payload), &s); err != nil {
			t.Fatal(err)
		}
		q := reportedStremioQuality(s)
		if tc.height == 0 && tc.bitrate == 0 {
			if q != nil {
				t.Errorf("unexpected quality: %+v", q)
			}
			continue
		}
		if q == nil || q.ResolutionHeight != tc.height || q.BitrateBps != tc.bitrate {
			t.Errorf("%s: %+v", tc.payload, q)
		}
	}
}
