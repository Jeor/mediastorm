package history

import (
	"strings"
	"time"

	"novastream/models"
)

const staleScrobbleDelaySessionAge = 2 * time.Hour

type scrobbleStartDelaySession struct {
	delay              time.Duration
	activePlaybackTime time.Duration
	lastPosition       float64
	lastUpdateAt       time.Time
	wasPlaying         bool
	started            bool
}

func scrobbleStartDelaySessionKey(userID string, update models.PlaybackProgressUpdate) string {
	return strings.Join([]string{
		userID,
		strings.ToLower(strings.TrimSpace(update.MediaType)),
		strings.ToLower(strings.TrimSpace(update.ItemID)),
	}, "\x00")
}

// shouldEmitRealtimeScrobbleLocked tracks active media playback for a pending
// start delay. The caller must hold s.mu.
func (s *Service) shouldEmitRealtimeScrobbleLocked(
	userID string,
	update models.PlaybackProgressUpdate,
	delay time.Duration,
	now time.Time,
) bool {
	if s.scrobbleStartDelaySessions == nil {
		s.scrobbleStartDelaySessions = make(map[string]*scrobbleStartDelaySession)
	}
	key := scrobbleStartDelaySessionKey(userID, update)

	for staleKey, session := range s.scrobbleStartDelaySessions {
		if session == nil || now.Sub(session.lastUpdateAt) > staleScrobbleDelaySessionAge {
			delete(s.scrobbleStartDelaySessions, staleKey)
		}
	}

	session := s.scrobbleStartDelaySessions[key]
	if update.PlaybackEnded {
		delete(s.scrobbleStartDelaySessions, key)
		return (session != nil && session.started) || (session == nil && delay <= 0)
	}

	if session == nil {
		session = &scrobbleStartDelaySession{delay: delay, started: delay <= 0}
		s.scrobbleStartDelaySessions[key] = session
	} else if session.wasPlaying && session.lastUpdateAt.Before(now) {
		mediaAdvance := update.Position - session.lastPosition
		elapsed := now.Sub(session.lastUpdateAt)
		if mediaAdvance > 0 && elapsed > 0 {
			if maxAdvance := elapsed.Seconds(); mediaAdvance > maxAdvance {
				mediaAdvance = maxAdvance
			}
			session.activePlaybackTime += time.Duration(mediaAdvance * float64(time.Second))
		}
	}

	session.lastPosition = update.Position
	session.lastUpdateAt = now
	session.wasPlaying = !update.IsPaused && !update.IsBuffering
	if !session.started && session.wasPlaying && session.activePlaybackTime >= session.delay {
		session.started = true
	}
	return session.started
}

func (s *Service) resolveRealtimeScrobbleStartDelay(userID string, update models.PlaybackProgressUpdate) time.Duration {
	s.mu.RLock()
	if session := s.scrobbleStartDelaySessions[scrobbleStartDelaySessionKey(userID, update)]; session != nil {
		delay := session.delay
		s.mu.RUnlock()
		return delay
	}
	resolver := s.scrobbleStartDelayResolver
	s.mu.RUnlock()
	if resolver == nil {
		return 0
	}
	return resolver(userID, update.ClientID)
}

func (s *Service) clearScrobbleStartDelaySessionLocked(userID string, update models.PlaybackProgressUpdate) {
	delete(s.scrobbleStartDelaySessions, scrobbleStartDelaySessionKey(userID, update))
}
