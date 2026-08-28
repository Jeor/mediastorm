package models

import "time"

// RealtimeScrobbleSession is a durable record of a provider-side playing or
// paused session. It survives backend restarts so abandoned remote sessions can
// be reconciled against the active-stream dashboard.
type RealtimeScrobbleSession struct {
	Provider       string
	UserID         string
	MediaType      string
	ItemID         string
	RemoteKey      string
	State          string
	PercentWatched float64
	Update         PlaybackProgressUpdate
	StartedAt      time.Time
	UpdatedAt      time.Time
}
