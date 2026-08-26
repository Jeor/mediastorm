package debrid

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAIOStreamsSearchSkipsStatisticAndExternalURLOnlyEntries(t *testing.T) {
	var capturedPath string
	client := newStubClient(func(r *http.Request) (*http.Response, error) {
		capturedPath = r.URL.Path
		return jsonResponse(http.StatusOK, `{
			"streams": [
				{
					"name": "🔍 Removal Reasons",
					"description": "📌 Title Matching (4)",
					"externalUrl": "https://github.com/Viren070/AIOStreams",
					"streamData": {"type": "statistic"}
				},
				{
					"name": "External details",
					"description": "Provider details page",
					"externalUrl": "https://example.test/details"
				},
				{
					"name": "AIOStreams 1080p",
					"description": "🎬 The Movie\n📡 Torrentio\n🎥 WEB-DL\n📦 2.5 GB",
					"url": "https://cdn.example.test/movie.mkv",
					"behaviorHints": {
						"filename": "The.Movie.2024.1080p.WEB-DL.mkv",
						"videoSize": 2684354560
					}
				}
			]
		}`), nil
	})

	scraper := NewAIOStreamsScraper("https://aiostreams.test/stremio/user/config/manifest.json", "AIOStreams", false, client)
	results, err := scraper.Search(context.Background(), SearchRequest{
		Parsed: ParsedQuery{
			Title:     "The Movie",
			Year:      2024,
			MediaType: MediaTypeMovie,
		},
		IMDBID: "tt1234567",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if capturedPath != "/stremio/user/config/stream/movie/tt1234567.json" {
		t.Fatalf("expected AIOStreams stream path, got %q", capturedPath)
	}
	if len(results) != 1 {
		t.Fatalf("expected only the playable URL result, got %d", len(results))
	}
	if results[0].TorrentURL != "https://cdn.example.test/movie.mkv" {
		t.Fatalf("expected playable URL, got %q", results[0].TorrentURL)
	}
	if results[0].Attributes["stream_url"] != "https://cdn.example.test/movie.mkv" {
		t.Fatalf("expected stream_url attribute to be preserved, got %#v", results[0].Attributes)
	}
	if strings.Contains(results[0].Title, "Removal Reasons") {
		t.Fatalf("statistic entry was returned as a playable result: %#v", results[0])
	}
}

func TestAIOStreamsCurrentFormatParsingAndPassthrough(t *testing.T) {
	rawName := "   4K ‍‍    ‍‍‍⚡‍‍\n  〈Bluray〉‍     ‍\n  ★★☆            "
	rawDescription := "☁︎  The Incredibles (2004) \n▣  HEVC  ✦  DV · HDR10  \n♬  DD+  ♯  7.1 \n◈ 12.9 GB · 14.9 ᴹᵇᵖˢ \n⛉ [RD] Library · hallowed\n✓ ᴇɴ · ᴍᴜʟᴛɪ · sᴜʙ (ꜰʀ)  »  ₅₀₅₀"
	client := newStubClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"streams": [
				{
					"name": "   4K ‍‍    ‍‍‍⚡‍‍\n  〈Bluray〉‍     ‍\n  ★★☆            ",
					"description": "☁︎  The Incredibles (2004) \n▣  HEVC  ✦  DV · HDR10  \n♬  DD+  ♯  7.1 \n◈ 12.9 GB · 14.9 ᴹᵇᵖˢ \n⛉ [RD] Library · hallowed\n✓ ᴇɴ · ᴍᴜʟᴛɪ · sᴜʙ (ꜰʀ)  »  ₅₀₅₀",
					"url": "https://cdn.example.test/incredibles.mkv",
					"behaviorHints": {"filename": "The.Incredibles.2004.2160p.DV.HDR10.mkv"}
				}
			]
		}`), nil
	})

	scraper := NewAIOStreamsScraper("https://aiostreams.test/config/manifest.json", "AIOStreams", true, client)
	results, err := scraper.Search(context.Background(), SearchRequest{
		Parsed: ParsedQuery{Title: "The Incredibles", Year: 2004, MediaType: MediaTypeMovie},
		IMDBID: "tt0317705",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	result := results[0]
	if result.Attributes["passthrough_format"] != "true" {
		t.Fatalf("passthrough flag missing: %#v", result.Attributes)
	}
	if result.Attributes["raw_name"] != rawName || result.Attributes["raw_description"] != rawDescription {
		t.Fatalf("raw display strings changed: name=%q description=%q", result.Attributes["raw_name"], result.Attributes["raw_description"])
	}
	for key, want := range map[string]string{
		"tracker":    "Library",
		"source":     "BluRay",
		"hdr":        "DV | HDR10",
		"codec":      "HEVC",
		"audio":      "DD+",
		"resolution": "2160p",
		"languages":  "English,Multi",
	} {
		if got := result.Attributes[key]; got != want {
			t.Errorf("attribute %s = %q, want %q", key, got, want)
		}
	}
	if result.MetaName != "The Incredibles (2004)" {
		t.Fatalf("MetaName = %q, want marker-free title", result.MetaName)
	}
	if result.SizeBytes < 12*1024*1024*1024 {
		t.Fatalf("SizeBytes = %d, expected parsed 12.9 GB", result.SizeBytes)
	}
}

func TestParseAIODescriptionSupportsLegacyFormat(t *testing.T) {
	parsed := parseAIODescription(
		"AIOStreams 1080p",
		"🎬 The Movie\n📡 Torrentio\n🎥 WEB-DL\n📺 HDR | DV\n🎞️ HEVC\n🎧 Atmos | DTS-HD MA\n📦 2.5 GB\n🇬🇧",
	)

	if parsed.provider != "Torrentio" || parsed.source != "WEB-DL" || parsed.hdr != "HDR | DV" || parsed.codec != "HEVC" || parsed.audio != "Atmos | DTS-HD MA" {
		t.Fatalf("legacy format parsed incorrectly: %#v", parsed)
	}
	if !reflect.DeepEqual(parsed.languages, []string{"English"}) {
		t.Fatalf("languages = %#v, want English", parsed.languages)
	}
	if parsed.sizeBytes < 2*1024*1024*1024 {
		t.Fatalf("sizeBytes = %d, expected parsed 2.5 GB", parsed.sizeBytes)
	}
}

func TestParseAIODescriptionSupportsCurrentCompactFormat(t *testing.T) {
	parsed := parseAIODescription(
		"1080P‍‍    ‍‍‍⚡‍‍\n  〈Web‍-‍dl〉‍\n  ★★★★★",
		"✎  Widow's Bay  s₀₁·ᴇ₀₁\n▣  AVC  ✦  10bit · DV · HDR10  ♬  AAC · DD  ♯  5.1 · 2.0\n❖ 2.22 / 36.9 GB · 7.15 ᴹᵇᵖˢ\n⛉ [RD] ComeTorz\n✓ ᴇɴ · sᴜʙ (ꜰʀ)",
	)

	if parsed.provider != "ComeTorz" || parsed.source != "WEB-DL" || parsed.hdr != "DV | HDR10" || parsed.codec != "AVC" || parsed.audio != "AAC · DD" {
		t.Fatalf("compact current format parsed incorrectly: %#v", parsed)
	}
	if !reflect.DeepEqual(parsed.languages, []string{"English"}) {
		t.Fatalf("languages = %#v, want audio language English only", parsed.languages)
	}
	if parsed.sizeBytes < 2*1024*1024*1024 {
		t.Fatalf("sizeBytes = %d, expected first reported size of 2.22 GB", parsed.sizeBytes)
	}
}
