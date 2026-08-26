package updates

import (
	"testing"
	"time"
)

func TestParseReleaseTag(t *testing.T) {
	version, build := ParseReleaseTag("v1.5.0-20260618")
	if version != "1.5.0" || build != "20260618" {
		t.Fatalf("ParseReleaseTag = %q, %q", version, build)
	}
}

func TestIsNewerVersion(t *testing.T) {
	if !IsNewer("1.5.0", "20260618", "1.6.0", "20260601") {
		t.Fatal("expected newer semantic version to be available")
	}
	if !IsNewer("1.5.0", "20260618", "1.5.0", "20260619") {
		t.Fatal("expected newer build to be available")
	}
	if IsNewer("1.5.0", "20260618", "1.5.0", "20260618") {
		t.Fatal("did not expect same version/build to be available")
	}
	if IsNewer("unknown", "", "1.5.0", "20260618") {
		t.Fatal("did not expect unknown current version to report available")
	}
}

func TestCacheTTLRefreshesReleaseMissingRequestedAPK(t *testing.T) {
	service := &Service{ttl: 30 * time.Minute}
	release := &githubRelease{}

	if got := service.cacheTTL(release, "mediastorm-tv-"); got != incompleteReleaseTTL {
		t.Fatalf("cacheTTL = %s, want %s", got, incompleteReleaseTTL)
	}
}

func TestCacheTTLKeepsNormalTTLWhenRequestedAPKExists(t *testing.T) {
	service := &Service{ttl: 30 * time.Minute}
	release := &githubRelease{}
	release.Assets = append(release.Assets, githubReleaseAsset{
		Name:               "mediastorm-tv-1.5.0-20260825.apk",
		BrowserDownloadURL: "https://example.com/mediastorm-tv.apk",
	})

	if got := service.cacheTTL(release, "mediastorm-tv-"); got != service.ttl {
		t.Fatalf("cacheTTL = %s, want %s", got, service.ttl)
	}
}

func TestFillLatestWaitsForRequestedAPK(t *testing.T) {
	status := ComponentStatus{CurrentVersion: "1.5.0", CurrentBuildID: "20260824"}
	release := &githubRelease{TagName: "v1.5.0-20260825"}

	got := fillLatest(status, release, "1.5.0", "20260825", "mediastorm-tv-")
	if got.UpdateAvailable {
		t.Fatal("expected update to remain unavailable until the requested APK exists")
	}
	if got.APKDownloadURL != "" {
		t.Fatalf("APKDownloadURL = %q, want empty", got.APKDownloadURL)
	}
}

func TestFillLatestMakesUpdateAvailableWithRequestedAPK(t *testing.T) {
	status := ComponentStatus{CurrentVersion: "1.5.0", CurrentBuildID: "20260824"}
	release := &githubRelease{TagName: "v1.5.0-20260825"}
	release.Assets = append(release.Assets, githubReleaseAsset{
		Name:               "mediastorm-tv-1.5.0-20260825.apk",
		BrowserDownloadURL: "https://example.com/mediastorm-tv.apk",
	})

	got := fillLatest(status, release, "1.5.0", "20260825", "mediastorm-tv-")
	if !got.UpdateAvailable {
		t.Fatal("expected update to become available once the requested APK exists")
	}
	if got.APKDownloadURL != "https://example.com/mediastorm-tv.apk" {
		t.Fatalf("APKDownloadURL = %q", got.APKDownloadURL)
	}
}
