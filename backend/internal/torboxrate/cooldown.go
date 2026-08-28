package torboxrate

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultDelay     = 5 * time.Second
	maximumDelay     = 2 * time.Minute
	minimumDelay     = 250 * time.Millisecond
	fallbackResetAge = 5 * time.Minute
)

var retryInPattern = regexp.MustCompile(`(?i)retry(?: again)? in\s*([0-9]+(?:\.[0-9]+)?)\s*(ms|milliseconds?|s|secs?|seconds?|m|mins?|minutes?)?`)

// Downloads coordinates cooldowns across every backend consumer of TorBox's
// download CDN. TorBox limits apply to the shared download link/account, not
// to an individual playback, probe, or thumbnail job.
var Downloads = &Cooldown{}

type Cooldown struct {
	mu            sync.Mutex
	until         time.Time
	lastLimited   time.Time
	fallbackDelay time.Duration
}

func IsDownloadURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host == "tb-cdn.io" || strings.HasSuffix(host, ".tb-cdn.io")
}

// Wait blocks only while a previously observed TorBox 429 is cooling down.
func (c *Cooldown) Wait(ctx context.Context, rawURL string) error {
	if c == nil || !IsDownloadURL(rawURL) {
		return nil
	}
	for {
		c.mu.Lock()
		waitFor := time.Until(c.until)
		c.mu.Unlock()
		if waitFor <= 0 {
			return nil
		}
		timer := time.NewTimer(waitFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Record extends the shared cooldown using Retry-After or TorBox's response
// text (for example, "Too many requests, retry in 4s").
func (c *Cooldown) Record(rawURL, retryAfter string, body []byte) time.Duration {
	if c == nil || !IsDownloadURL(rawURL) {
		return 0
	}
	now := time.Now()
	delay, explicit := retryDelay(retryAfter, body, now)

	c.mu.Lock()
	defer c.mu.Unlock()
	if !explicit {
		if c.lastLimited.IsZero() || now.Sub(c.lastLimited) > fallbackResetAge {
			c.fallbackDelay = defaultDelay
		} else if c.fallbackDelay < maximumDelay {
			c.fallbackDelay *= 2
			if c.fallbackDelay > maximumDelay {
				c.fallbackDelay = maximumDelay
			}
		}
		delay = c.fallbackDelay
	}
	if delay < minimumDelay {
		delay = minimumDelay
	}
	if delay > maximumDelay {
		delay = maximumDelay
	}
	c.lastLimited = now
	newUntil := now.Add(delay)
	if newUntil.After(c.until) {
		c.until = newUntil
	}
	return time.Until(c.until)
}

func retryDelay(retryAfter string, body []byte, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(retryAfter)
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds * float64(time.Second)), true
	}
	if when, err := http.ParseTime(value); err == nil {
		return when.Sub(now), true
	}

	match := retryInPattern.FindStringSubmatch(string(body))
	if len(match) < 2 {
		return 0, false
	}
	amount, err := strconv.ParseFloat(match[1], 64)
	if err != nil || amount < 0 {
		return 0, false
	}
	unit := strings.ToLower(match[2])
	scale := time.Second
	if strings.HasPrefix(unit, "ms") || strings.HasPrefix(unit, "millisecond") {
		scale = time.Millisecond
	} else if strings.HasPrefix(unit, "m") && !strings.HasPrefix(unit, "ms") {
		scale = time.Minute
	}
	return time.Duration(amount * float64(scale)), true
}
