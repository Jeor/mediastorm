package sports

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Route guides are organizer diagrams, not live rider positions or route geometry.
type CyclingRouteGuide struct {
	State      string          `json:"state"`
	Source     cyclingEvidence `json:"source"`
	SourceURL  string          `json:"sourceUrl"`
	MapURL     string          `json:"mapUrl,omitempty"`
	ProfileURL string          `json:"profileUrl,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}
type cyclingGuideCache struct {
	guide   CyclingRouteGuide
	expires time.Time
}

func giroGuideURL(value string, year, number int) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host == "www.giroditalia.it" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.HasPrefix(u.Path, fmt.Sprintf("/en/tappe/stage-%d-of-the-giro-ditalia-%d-", number, year)) && !strings.Contains(u.Path, "..") && u.RawPath == ""
}
func giroGuideImage(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "static2.giroditalia.it" || u.User != nil || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.Path, "..") || !strings.HasPrefix(u.Path, "/wp-content/uploads/") {
		return false
	}
	path := strings.ToLower(u.Path)
	return strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".webp")
}
func normalizeGiroGuide(doc *html.Node, sourceURL string, year, number int, now time.Time) (CyclingRouteGuide, error) {
	guide := CyclingRouteGuide{State: "unavailable", Source: giroEvidence(now), SourceURL: sourceURL}
	if !giroGuideURL(sourceURL, year, number) || !giroEdition(doc, year) {
		return guide, fmt.Errorf("Giro route edition mismatch")
	}
	canonical := giroNodes(doc, func(n *html.Node) bool { return n.Data == "link" && giroAttr(n, "rel") == "canonical" })
	titles := giroNodes(doc, func(n *html.Node) bool { return n.Data == "title" })
	if len(canonical) != 1 || giroAttr(canonical[0], "href") != sourceURL || len(titles) != 1 || !strings.HasPrefix(giroText(titles[0]), fmt.Sprintf("Stage %d of the Giro", number)) {
		return guide, fmt.Errorf("Giro route stage mismatch")
	}
	for class, target := range map[string]*string{"js-tab-planimetria": &guide.MapURL, "js-tab-info-altrimetria": &guide.ProfileURL} {
		panels := giroClass(doc, class)
		if len(panels) != 1 {
			continue
		}
		// The organizer's first slide is the primary diagram. Further circuit or
		// finish diagrams remain available through the complete official guide.
		slides := giroClass(panels[0], "single-slide")
		if len(slides) == 0 {
			continue
		}
		images := giroNodes(slides[0], func(n *html.Node) bool { return n.Data == "img" })
		if len(images) == 1 && giroGuideImage(giroAttr(images[0], "src")) {
			*target = giroAttr(images[0], "src")
		}
	}
	if guide.MapURL == "" && guide.ProfileURL == "" {
		return guide, fmt.Errorf("Giro route images unavailable")
	}
	guide.State = "available"
	return guide, nil
}

// Called under cycling.detailMu; guide failures cannot replace classifications.
func (s *Service) enrichCyclingGuide(ctx context.Context, stage *CyclingStage, year, number int, now time.Time) {
	if stage.RouteGuide == nil {
		return
	}
	cache := &s.cycling
	if cache.guides == nil {
		cache.guides = map[string]cyclingGuideCache{}
	}
	old, exists := cache.guides[stage.ID]
	if exists && now.Before(old.expires) {
		guide := old.guide
		stage.RouteGuide = &guide
		return
	}
	sourceURL := stage.RouteGuide.SourceURL
	guide := CyclingRouteGuide{State: "unavailable", Source: giroEvidence(now), SourceURL: sourceURL, Reason: "The organizer route guide could not be verified. Open the official stage guide for route information."}
	var err error
	parts := strings.Split(stage.ID, ":")
	if len(parts) != 4 {
		return
	}
	site, aso := asoGuideSites[parts[1]]
	if !aso && (!strings.HasPrefix(stage.ID, "rcs:giro:") || !giroGuideURL(sourceURL, year, number)) {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var doc *html.Node
	if aso {
		guide.Source = cyclingSource(now, 0)
		doc, err = s.asoGuideHTML(requestCtx, site, number)
		if err == nil {
			guide, err = normalizeASOGuide(doc, site, year, number, now)
		}
	} else {
		doc, err = s.giroHTML(requestCtx, strings.TrimPrefix(sourceURL, "https://www.giroditalia.it/en/"))
		if err == nil {
			guide, err = normalizeGiroGuide(doc, sourceURL, year, number, now)
		}
	}
	ttl := 30 * time.Minute
	if err != nil {
		ttl = 30 * time.Second
		if exists && (old.guide.MapURL != "" || old.guide.ProfileURL != "") {
			guide = old.guide
			guide.State = "stale"
		}
		guide.Reason = "The organizer route guide could not be refreshed. Open the official stage guide for current route information."
	}
	if len(cache.guides) >= 40 {
		oldest := ""
		var expiry time.Time
		for key, item := range cache.guides {
			if oldest == "" || item.expires.Before(expiry) {
				oldest = key
				expiry = item.expires
			}
		}
		delete(cache.guides, oldest)
	}
	cache.guides[stage.ID] = cyclingGuideCache{guide: guide, expires: now.Add(ttl)}
	stage.RouteGuide = &guide
}
