package sports

import (
	"context"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type asoGuideSite struct{ host, title, namespace string }

var asoGuideSites = map[string]asoGuideSite{
	"tour":        {"www.letour.fr", "Tour de France", "tdf"},
	"tour-femmes": {"www.letourfemmes.fr", "Tour de France Femmes", "trf"},
	"paris-nice":  {"www.paris-nice.fr", "Paris-Nice", "pnc"},
	"vuelta":      {"www.lavuelta.es", "La Vuelta", "vue"},
}

func asoGuideImage(value string, site asoGuideSite) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "img.aso.fr" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.Path, "..") {
		return false
	}
	for _, format := range []string{"jpg", "png", "webp"} {
		if strings.HasPrefix(u.Path, "/core_app/img-cycling-"+site.namespace+"-"+format+"/") && len(strings.Split(strings.Trim(u.Path, "/"), "/")) >= 5 {
			return true
		}
	}
	return false
}
func normalizeASOGuide(doc *html.Node, site asoGuideSite, year, number int, now time.Time) (CyclingRouteGuide, error) {
	sourceURL := fmt.Sprintf("https://%s/en/stage-%d", site.host, number)
	guide := CyclingRouteGuide{State: "unavailable", Source: cyclingSource(now, 0), SourceURL: sourceURL}
	years := giroNodes(doc, func(n *html.Node) bool { return n.Data == "meta" && giroAttr(n, "name") == "year" })
	urls := giroNodes(doc, func(n *html.Node) bool { return n.Data == "meta" && giroAttr(n, "property") == "og:url" })
	titles := giroNodes(doc, func(n *html.Node) bool { return n.Data == "title" })
	if len(years) != 1 || giroAttr(years[0], "content") != strconv.Itoa(year) || len(urls) != 1 || giroAttr(urls[0], "content") != sourceURL || len(titles) != 1 || !strings.HasPrefix(giroText(titles[0]), fmt.Sprintf("Stage %d - ", number)) || !strings.HasSuffix(giroText(titles[0]), fmt.Sprintf(" - %s %d", site.title, year)) {
		return guide, fmt.Errorf("ASO guide identity mismatch")
	}
	for id, target := range map[string]*string{"profil": &guide.ProfileURL, "map": &guide.MapURL} {
		panels := giroNodes(doc, func(n *html.Node) bool { return giroAttr(n, "id") == id })
		if len(panels) != 1 {
			continue
		}
		candidates := map[string]bool{}
		for _, img := range giroClass(panels[0], "sporting__content__img") {
			value := giroAttr(img, "data-src")
			if img.Data == "img" && asoGuideImage(value, site) {
				candidates[value] = true
			}
		}
		if id == "map" && site.namespace == "tdf" {
			for _, a := range giroNodes(panels[0], func(n *html.Node) bool { return n.Data == "a" }) {
				value := giroAttr(a, "href")
				if asoGuideImage(value, site) && giroText(a) == "Download the map" {
					candidates[value] = true
				}
			}
		}
		if len(candidates) == 1 {
			for value := range candidates {
				*target = value
			}
		}
	}
	if guide.MapURL == "" && guide.ProfileURL == "" {
		guide.Reason = "Static route images are not available. Open the official stage guide for the organizer’s route view."
		return guide, nil
	}
	guide.State = "available"
	return guide, nil
}
func (s *Service) asoGuideHTML(ctx context.Context, site asoGuideSite, number int) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/en/stage-%d", site.host, number), nil)
	if err != nil {
		return nil, err
	}
	client := *s.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.Scheme != "https" || next.URL.Host != site.host || next.URL.User != nil {
			return fmt.Errorf("untrusted ASO redirect")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ASO guide status %d", resp.StatusCode)
	}
	const limit = 3 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("ASO guide exceeds size limit")
	}
	return html.Parse(strings.NewReader(string(body)))
}
