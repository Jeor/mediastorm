package scheduler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	mdblistHistoryMaxRetries   = 3
	mdblistHistoryMaxRetryWait = 5 * time.Minute
)

func waitMDBListRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mdblistRetryDelay(header string, now time.Time, attempt int) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(header), 10, 64); err == nil && seconds >= 0 {
		// Avoid overflow for untrusted header values.
		if seconds > int64(mdblistHistoryMaxRetryWait/time.Second) {
			return mdblistHistoryMaxRetryWait + time.Second
		}
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(header); err == nil {
		return max(0, retryAt.Sub(now))
	}
	return 30 * time.Second * time.Duration(1<<attempt)
}

// Retry the same offset only. Long account quota waits finish with an error
// instead of leaving an automation running until the daily reset.
func fetchMDBListHistoryPage(ctx context.Context, client *http.Client, requestURL, taskID string, offset int, wait func(context.Context, time.Duration) error) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create MDBList history request: %w", mdblistRequestError(err))
		}
		req.Header.Set("User-Agent", "mediastorm/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch MDBList history: %w", mdblistRequestError(err))
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			resp.Body.Close()
			return nil, fmt.Errorf("MDBList history request failed (HTTP %d, offset %d)", resp.StatusCode, offset)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		delay := mdblistRetryDelay(resp.Header.Get("Retry-After"), time.Now(), attempt)
		if strings.Contains(string(body), "Daily API limit exceeded") || delay > mdblistHistoryMaxRetryWait {
			return nil, fmt.Errorf("MDBList history request failed (HTTP 429, offset %d): account quota exceeded or retry wait exceeds 5 minutes; retry after MDBList's quota resets", offset)
		}
		if attempt >= mdblistHistoryMaxRetries {
			return nil, fmt.Errorf("MDBList history request failed (HTTP 429, offset %d) after %d retries", offset, attempt)
		}
		log.Printf("[scheduler] MDBList history rate limited task=%q offset=%d retry=%d wait=%s", taskID, offset, attempt+1, delay)
		if err := wait(ctx, delay); err != nil {
			return nil, fmt.Errorf("wait to retry MDBList history (offset %d): %w", offset, err)
		}
	}
}
