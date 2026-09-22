package mediaidentity

import "time"

// Filters consult identities for every candidate. Log actual use once per
// source/season per ten minutes, rather than once per result. A change from
// automatic to fallback (or vice versa) is always distinguishable.
func (d *discoveryService) logSelection(titleID string, season, episode int, mapped AnthologyEpisode, source string) {
	key := discoveryKey(titleID, season) + "/" + source
	now := time.Now()
	d.mu.Lock()
	if until := d.selections[key]; now.Before(until) {
		d.mu.Unlock()
		return
	}
	if len(d.selections) >= 512 {
		for k, until := range d.selections {
			if !now.Before(until) {
				delete(d.selections, k)
			}
		}
		if len(d.selections) >= 512 {
			for k := range d.selections {
				delete(d.selections, k)
				break
			}
		}
	}
	d.selections[key] = now.Add(10 * time.Minute)
	d.mu.Unlock()
	d.logf("[mediaidentity] mapping selected title=%s catalog=S%02dE%02d source=%s imdb=%s provider=S%02dE%02d", titleID, season, episode, source, mapped.IMDBID, mapped.Season, mapped.Episode)
}
