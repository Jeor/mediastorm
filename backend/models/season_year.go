package models

// SeriesSeasonPremiereYear returns the year of the requested season's first
// episode. Do not borrow dates from other seasons or later episodes: those can
// coincide with a reboot's premiere and make an unrelated release look valid.
func SeriesSeasonPremiereYear(seasons []SeriesSeason, seasonNumber int) int {
	if seasonNumber <= 0 {
		return 0
	}
	for _, season := range seasons {
		if season.Number != seasonNumber {
			continue
		}
		for _, episode := range season.Episodes {
			if episode.EpisodeNumber != 1 {
				continue
			}
			if date, ok := parseReleaseDate(episode.AiredDate); ok {
				return date.Year()
			}
			if date, ok := parseReleaseDate(episode.AiredDateTimeUTC); ok {
				return date.Year()
			}
		}
	}
	return 0
}
