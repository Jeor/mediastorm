package sports

import (
	"golang.org/x/net/html"
	"os"
	"strings"
	"testing"
	"time"
)

func TestASOGuideIdentityAndAssets(t *testing.T) {
	for _, slug := range []string{"tour", "tour-femmes", "paris-nice", "vuelta"} {
		t.Run(slug, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/aso-" + slug + "-guide.html")
			if err != nil {
				t.Fatal(err)
			}
			doc, _ := html.Parse(strings.NewReader(string(raw)))
			site := asoGuideSites[slug]
			guide, err := normalizeASOGuide(doc, site, 2026, 1, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if (guide.MapURL != "") != (slug == "tour" || slug == "tour-femmes") || (guide.ProfileURL != "") != (slug != "vuelta") {
				t.Fatalf("wrong assets %+v", guide)
			}
			if _, err = normalizeASOGuide(doc, site, 2025, 1, time.Now()); err == nil {
				t.Fatal("wrong edition accepted")
			}
			if _, err = normalizeASOGuide(doc, site, 2026, 2, time.Now()); err == nil {
				t.Fatal("wrong stage accepted")
			}
			for _, bad := range []string{"https://evil.test/core_app/img-cycling-tdf-jpg/a/b/c", "https://img.aso.fr/core_app/img-cycling-tdf-jpg/../a/b", "/img/stage/default-profil.png"} {
				if asoGuideImage(bad, site) {
					t.Fatal("unsafe image accepted")
				}
			}
		})
	}
}
