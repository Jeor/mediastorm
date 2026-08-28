package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// PlaybackLatencySample is one click→first-frame measurement of the VOD
// playback path. All timestamps are server wall-clock.
//
//	t0 ClientRequestedAt       - client POST /playback/prequeue arrived
//	t1 PrequeueReadyAt         - prequeue worker marked the entry ready
//	t2 HLSSessionCreatedAt     - HLS session created (StreamStartTime)
//	t3 FirstSegmentReadyAt     - first media segment present on disk
//	t4 FirstSegmentSentAt      - first segment response began streaming
//
// Native (non-HLS) clients never reach t2+; their timeline ends at t1 and is
// reported by the PREQUEUE_LATENCY log line instead.
type PlaybackLatencySample struct {
	ID         string `json:"id"`
	PrequeueID string `json:"prequeueId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	TitleID    string `json:"titleId,omitempty"`
	UserID     string `json:"userId,omitempty"`
	ImdbID     string `json:"imdbId,omitempty"` // re-scoping a bench needs the same search context as the original click
	Year       int    `json:"year,omitempty"`
	TitleName  string `json:"titleName,omitempty"`
	MediaType  string `json:"mediaType,omitempty"`

	// ReleaseName is the selected release (e.g. "Her.2013.1080p.BluRay...-d3g")
	// so benchmark runs can be compared apples-to-apples per release. Backfilled
	// from the prequeue's selected result once resolution makes its choice.
	ReleaseName string `json:"releaseName,omitempty"`

	// Candidates lists every candidate the resolution race tried for this
	// prequeue, with its outcome and wall-clock duration, so dead-release
	// rejections (probe_rejected / articles_unavailable) are measurable even
	// when they lose the race to a faster candidate.
	Candidates []PlaybackCandidateAttempt `json:"candidates,omitempty"`

	ServiceType     string `json:"serviceType,omitempty"`     // "usenet" | "debrid" | ""
	ServiceProvider string `json:"serviceProvider,omitempty"` // indexer / debrid provider when known

	ClientRequestedAt   time.Time `json:"clientRequestedAt,omitempty"`   // t0
	PrequeueReadyAt     time.Time `json:"prequeueReadyAt,omitempty"`     // t1
	HLSSessionCreatedAt time.Time `json:"hlsSessionCreatedAt,omitempty"` // t2
	FirstSegmentReadyAt time.Time `json:"firstSegmentReadyAt,omitempty"` // t3
	FirstSegmentSentAt  time.Time `json:"firstSegmentSentAt,omitempty"`  // t4

	// Derived durations in milliseconds. -1 when the phase could not be
	// measured (missing endpoints).
	TotalMs        int64 `json:"totalMs"`        // t0→t4
	PrequeueMs     int64 `json:"prequeueMs"`     // t0→t1 (search+resolve+probe)
	HLSCreateMs    int64 `json:"hlsCreateMs"`    // t1→t2 (session spin-up; from t0 when t1 unknown)
	FFmpegWarmupMs int64 `json:"ffmpegWarmupMs"` // t2→t3 (first segment on disk, incl. playlist fetch)
	ServeWaitMs    int64 `json:"serveWaitMs"`    // t3→t4 (segment file ready → response sent)

	Complete bool     `json:"complete"` // true when t0..t4 all present
	Notes    []string `json:"notes,omitempty"`
}

type pendingPrequeueTimes struct {
	requestedAt time.Time // t0
	readyAt     time.Time // t1
	titleID     string
	userID      string
	imdbID      string
	year        int
	titleName   string
	mediaType   string
	releaseName string
	// candidates maps 1-based feed index → last reported attempt outcome.
	candidates map[int]PlaybackCandidateAttempt
}

// PrequeueCandidateOutcome describes what happened to one prequeue candidate
// during the resolution race. Values are stable, kebab-case strings suitable
// for log greps and bench CSV summaries.
const (
	// PrequeueCandidateAdopted: fully validated; became the selected release.
	PrequeueCandidateAdopted = "adopted"
	// PrequeueCandidateProbeRejected: rejected by the cheap availability
	// probe before any full download (sampled segments missing from all providers).
	PrequeueCandidateProbeRejected = "probe_rejected"
	// PrequeueCandidateArticlesUnavailable: full resolve failed because the
	// articles are unavailable from every provider.
	PrequeueCandidateArticlesUnavailable = "articles_unavailable"
	// PrequeueCandidateFailed: resolve/probe/validation failed for another reason.
	PrequeueCandidateFailed = "failed"
	// PrequeueCandidateDeprioritized: unknown-track fallback; usable only when
	// nothing else validates (flipped to adopted if it ends up winning).
	PrequeueCandidateDeprioritized = "deprioritized"
	// PrequeueCandidateSuperseded: cancelled mid-flight when another candidate won.
	PrequeueCandidateSuperseded = "superseded"
)

// PlaybackCandidateAttempt is one candidate-resolution attempt of a prequeue.
// Index is the 1-based feed order; DurationMs covers the attempt itself
// (resolve + availability probe + validation), not the whole prequeue.
type PlaybackCandidateAttempt struct {
	Index       int    `json:"index"` // 1-based feed order
	ReleaseName string `json:"releaseName,omitempty"`
	ServiceType string `json:"serviceType,omitempty"`
	Outcome     string `json:"outcome"`
	DurationMs  int64  `json:"durationMs,omitempty"`
}

// PlaybackLatencyTracker correlates prequeue timestamps with HLS session
// timestamps and retains a rolling window of completed samples.
type PlaybackLatencyTracker struct {
	mu      sync.Mutex
	samples []PlaybackLatencySample // oldest → newest
	max     int
	pending map[string]*pendingPrequeueTimes
	seq     int64
}

func NewPlaybackLatencyTracker(maxSamples int) *PlaybackLatencyTracker {
	if maxSamples <= 0 {
		maxSamples = 400
	}
	return &PlaybackLatencyTracker{
		samples: make([]PlaybackLatencySample, 0, maxSamples),
		max:     maxSamples,
		pending: make(map[string]*pendingPrequeueTimes),
	}
}

// NotePrequeueRequested records t0 for a prequeue. A repeated request for the
// same prequeue (warm re-click) overwrites t0 so the latest click is measured.
func (t *PlaybackLatencyTracker) NotePrequeueRequested(prequeueID, titleID, userID, titleName, mediaType string) {
	if t == nil || prequeueID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for id, p := range t.pending {
		if now.Sub(p.requestedAt) > 30*time.Minute {
			delete(t.pending, id)
		}
	}
	p := t.pending[prequeueID]
	if p == nil {
		p = &pendingPrequeueTimes{}
		t.pending[prequeueID] = p
	}
	p.requestedAt = now
	p.titleID = titleID
	p.userID = userID
	p.titleName = titleName
	p.mediaType = mediaType
}

// NotePrequeueReady records t1 for a prequeue and, when the prequeue phase is
// measurable, emits the PREQUEUE_LATENCY log line (covers native clients whose
// timeline ends at ready).
func (t *PlaybackLatencyTracker) NotePrequeueReady(prequeueID string) {
	if t == nil || prequeueID == "" {
		return
	}
	t.mu.Lock()
	p := t.pending[prequeueID]
	if p == nil {
		p = &pendingPrequeueTimes{}
		t.pending[prequeueID] = p
	}
	p.readyAt = time.Now()
	requestedAt := p.requestedAt
	readyAt := p.readyAt
	titleName := p.titleName
	t.mu.Unlock()

	if !requestedAt.IsZero() {
		log.Printf("[latency] PREQUEUE_LATENCY prequeue=%dms title=%q prequeueId=%s",
			readyAt.Sub(requestedAt).Milliseconds(), titleName, prequeueID)
	}
}

// NotePrequeueRelease records the selected release for a prequeue once the
// resolution phase picks it, so latency samples name the exact release.
func (t *PlaybackLatencyTracker) NotePrequeueRelease(prequeueID, releaseName string) {
	if t == nil || prequeueID == "" || releaseName == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if p := t.pending[prequeueID]; p != nil {
		p.releaseName = releaseName
	}
}

// NotePrequeueMetadata records the extra search context (IMDb id + year) of a
// prequeue request so a later benchmark can re-scope the exact same search.
func (t *PlaybackLatencyTracker) NotePrequeueMetadata(prequeueID, imdbID string, year int) {
	if t == nil || prequeueID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if p := t.pending[prequeueID]; p != nil {
		p.imdbID = imdbID
		p.year = year
	}
}

// NotePrequeueCandidate records one candidate-resolution attempt of a prequeue
// so latency samples carry the per-candidate outcome + duration — the
// measurement surface for dead-release rejections ("probe_rejected in seconds"
// vs "articles_unavailable after minutes"). Upserts by index (each candidate
// reports once; an adopted fallback re-marks its earlier deprioritized attempt
// via MarkPrequeueCandidateAdopted). Also emits a grep-able [latency]
// PREQUEUE_CANDIDATE log line.
func (t *PlaybackLatencyTracker) NotePrequeueCandidate(prequeueID string, attempt PlaybackCandidateAttempt) {
	if t == nil || prequeueID == "" || attempt.Index <= 0 {
		return
	}
	t.mu.Lock()
	p := t.pending[prequeueID]
	if p == nil {
		p = &pendingPrequeueTimes{}
		t.pending[prequeueID] = p
	}
	if p.candidates == nil {
		p.candidates = make(map[int]PlaybackCandidateAttempt)
	}
	p.candidates[attempt.Index] = attempt
	t.mu.Unlock()

	log.Printf("[latency] PREQUEUE_CANDIDATE index=%d release=%q service=%s outcome=%s ms=%d prequeueId=%s",
		attempt.Index, attempt.ReleaseName, orUnknown(attempt.ServiceType), attempt.Outcome, attempt.DurationMs, prequeueID)
}

// MarkPrequeueCandidateAdopted flips a previously deprioritized (unknown-track
// fallback) candidate to adopted once it wins the race, preserving its measured
// duration. No-op when no attempt was recorded for the index.
func (t *PlaybackLatencyTracker) MarkPrequeueCandidateAdopted(prequeueID string, index int) {
	if t == nil || prequeueID == "" || index <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.pending[prequeueID]
	if p == nil {
		return
	}
	attempt, ok := p.candidates[index]
	if !ok {
		return
	}
	attempt.Outcome = PrequeueCandidateAdopted
	p.candidates[index] = attempt
}

// PrequeueTimes returns the recorded t0/t1 for a prequeue ID.
func (t *PlaybackLatencyTracker) PrequeueTimes(prequeueID string) (requestedAt, readyAt time.Time) {
	if t == nil || prequeueID == "" {
		return time.Time{}, time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.pending[prequeueID]
	if p == nil {
		return time.Time{}, time.Time{}
	}
	return p.requestedAt, p.readyAt
}

// Record stores a completed sample, fills derived durations, emits the
// PLAYBACK_LATENCY log line, and drops the pending prequeue state.
func (t *PlaybackLatencyTracker) Record(s PlaybackLatencySample) {
	if t == nil || s.FirstSegmentSentAt.IsZero() {
		return
	}
	prequeueID := s.PrequeueID
	t.mu.Lock()
	if p := t.pending[prequeueID]; p != nil {
		if s.ClientRequestedAt.IsZero() {
			s.ClientRequestedAt = p.requestedAt
		}
		if s.PrequeueReadyAt.IsZero() {
			s.PrequeueReadyAt = p.readyAt
		}
		if s.PrequeueID != "" && s.TitleID == "" {
			s.TitleID = p.titleID
		}
		if s.UserID == "" {
			s.UserID = p.userID
		}
		if s.ImdbID == "" {
			s.ImdbID = p.imdbID
		}
		if s.Year == 0 {
			s.Year = p.year
		}
		if s.TitleName == "" {
			s.TitleName = p.titleName
		}
		if s.ReleaseName == "" {
			s.ReleaseName = p.releaseName
		}
		if s.MediaType == "" {
			s.MediaType = p.mediaType
		}
		if len(s.Candidates) == 0 && len(p.candidates) > 0 {
			s.Candidates = sortedCandidateAttempts(p.candidates)
		}
		delete(t.pending, prequeueID)
	}
	t.mu.Unlock()
	t.storeAndLog(s)
}

// storeAndLog derives durations, appends the sample to the rolling window and
// emits the PLAYBACK_LATENCY line. Callers handle pending-state correlation.
func (t *PlaybackLatencyTracker) storeAndLog(s PlaybackLatencySample) {
	s = deriveLatencyDurations(s)
	t.mu.Lock()
	t.seq++
	s.ID = fmt.Sprintf("L%d", t.seq)
	t.samples = append(t.samples, s)
	if len(t.samples) > t.max {
		t.samples = t.samples[len(t.samples)-t.max:]
	}
	t.mu.Unlock()

	log.Printf("[latency] PLAYBACK_LATENCY total=%dms prequeue=%dms hlsCreate=%dms ffmpegWarmup=%dms serveWait=%dms service=%s title=%q complete=%v prequeueId=%s session=%s",
		s.TotalMs, s.PrequeueMs, s.HLSCreateMs, s.FFmpegWarmupMs, s.ServeWaitMs,
		orUnknown(s.ServiceType), s.TitleName, s.Complete, s.PrequeueID, s.SessionID)
}

// NotePrequeueOnlySample records a prequeue-phase-only sample (t0→t1) when no
// HLS session ever served a media segment (non-HLS stream, or the segment never
// materialized). It surfaces those iterations in the latency table as
// complete=false with a valid prequeueMs — precisely the phase the resolution
// work targets. No-op once a full sample has consumed the pending state.
func (t *PlaybackLatencyTracker) NotePrequeueOnlySample(prequeueID string) {
	if t == nil || prequeueID == "" {
		return
	}
	t.mu.Lock()
	p := t.pending[prequeueID]
	if p == nil {
		t.mu.Unlock()
		return // already recorded (a full sample consumed it) or unknown
	}
	delete(t.pending, prequeueID)
	candidates := sortedCandidateAttempts(p.candidates)
	t.mu.Unlock()

	t.storeAndLog(PlaybackLatencySample{
		PrequeueID:        prequeueID,
		TitleID:           p.titleID,
		UserID:            p.userID,
		ImdbID:            p.imdbID,
		Year:              p.year,
		TitleName:         p.titleName,
		MediaType:         p.mediaType,
		ReleaseName:       p.releaseName,
		ClientRequestedAt: p.requestedAt,
		PrequeueReadyAt:   p.readyAt,
		Candidates:        candidates,
		Notes:             []string{"no HLS session served a segment — prequeue phase only"},
	})
}

// NotePrequeueFailedSample records a prequeue that never reached ready (every
// candidate failed or was rejected, cancelled, ...) as a complete=false row so
// the all-candidates-dead path is measurable with its
// per-candidate attempts. Consumes the pending state like
// NotePrequeueOnlySample; no-op once a full sample already did.
func (t *PlaybackLatencyTracker) NotePrequeueFailedSample(prequeueID, reason string) {
	if t == nil || prequeueID == "" {
		return
	}
	t.mu.Lock()
	p := t.pending[prequeueID]
	if p == nil {
		t.mu.Unlock()
		return // already recorded (a full sample consumed it) or unknown
	}
	delete(t.pending, prequeueID)
	candidates := sortedCandidateAttempts(p.candidates)
	failedAt := time.Now()
	requestedAt := p.requestedAt
	t.mu.Unlock()

	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		trimmed = "prequeue failed"
	}
	t.storeAndLog(PlaybackLatencySample{
		PrequeueID:        prequeueID,
		TitleID:           p.titleID,
		UserID:            p.userID,
		ImdbID:            p.imdbID,
		Year:              p.year,
		TitleName:         p.titleName,
		MediaType:         p.mediaType,
		ReleaseName:       p.releaseName,
		ClientRequestedAt: requestedAt,
		PrequeueReadyAt:   failedAt,
		Candidates:        candidates,
		Notes:             []string{"prequeue failed: " + trimmed},
	})
	log.Printf("[latency] PREQUEUE_FAILURE prequeueMs=%dms reason=%q candidates=%d prequeueId=%s",
		time.Since(requestedAt).Milliseconds(), trimmed, len(candidates), prequeueID)
}

func sortedCandidateAttempts(m map[int]PlaybackCandidateAttempt) []PlaybackCandidateAttempt {
	if len(m) == 0 {
		return nil
	}
	out := make([]PlaybackCandidateAttempt, 0, len(m))
	for _, attempt := range m {
		out = append(out, attempt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func deriveLatencyDurations(s PlaybackLatencySample) PlaybackLatencySample {
	ms := func(from, to time.Time) int64 {
		if from.IsZero() || to.IsZero() {
			return -1
		}
		return to.Sub(from).Milliseconds()
	}
	s.PrequeueMs = ms(s.ClientRequestedAt, s.PrequeueReadyAt)
	// Ready before the click (shared prewarm work / re-click on an already-ready
	// entry) means the prequeue phase cost the client nothing.
	if s.PrequeueMs < 0 && !s.ClientRequestedAt.IsZero() && !s.PrequeueReadyAt.IsZero() {
		s.PrequeueMs = 0
	}
	// HLS creation is normally after ready (web path). For prequeue-created
	// sessions (HDR/audio transcode) it can precede ready; measure from t0.
	hlsFrom := s.PrequeueReadyAt
	if hlsFrom.IsZero() {
		hlsFrom = s.ClientRequestedAt
	}
	s.HLSCreateMs = ms(hlsFrom, s.HLSSessionCreatedAt)
	if s.HLSCreateMs < 0 && !s.HLSSessionCreatedAt.IsZero() {
		s.HLSCreateMs = -2 // session exists but t1 unknown
	}
	s.FFmpegWarmupMs = ms(s.HLSSessionCreatedAt, s.FirstSegmentReadyAt)
	s.ServeWaitMs = ms(s.FirstSegmentReadyAt, s.FirstSegmentSentAt)
	s.TotalMs = ms(s.ClientRequestedAt, s.FirstSegmentSentAt)
	s.Complete = !s.ClientRequestedAt.IsZero() && !s.PrequeueReadyAt.IsZero() &&
		!s.HLSSessionCreatedAt.IsZero() && !s.FirstSegmentReadyAt.IsZero() && !s.FirstSegmentSentAt.IsZero()
	if !s.ClientRequestedAt.IsZero() && s.FirstSegmentSentAt.IsZero() {
		s.Notes = append(s.Notes, "no first segment served")
	}
	if s.ClientRequestedAt.IsZero() {
		s.Notes = append(s.Notes, "no prequeue correlation (direct HLS start)")
	}
	return s
}

// Latest returns up to n samples, newest first.
func (t *PlaybackLatencyTracker) Latest(n int) []PlaybackLatencySample {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]PlaybackLatencySample, 0, n)
	for i := len(t.samples) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, t.samples[i])
	}
	return out
}

func (t *PlaybackLatencyTracker) Count() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.samples)
}

// ClearSamples drops the sample window (pending prequeue state is retained so
// in-flight playbacks still complete a sample).
func (t *PlaybackLatencyTracker) ClearSamples() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.samples = nil
}

type latencyStat struct {
	Min int64 `json:"minMs"`
	P50 int64 `json:"p50Ms"`
	P95 int64 `json:"p95Ms"`
	Max int64 `json:"maxMs"`
	Avg int64 `json:"avgMs"`
	N   int   `json:"n"`
}

type LatencyStats struct {
	TotalMs        latencyStat `json:"totalMs"`
	PrequeueMs     latencyStat `json:"prequeueMs"`
	HLSCreateMs    latencyStat `json:"hlsCreateMs"`
	FFmpegWarmupMs latencyStat `json:"ffmpegWarmupMs"`
	ServeWaitMs    latencyStat `json:"serveWaitMs"`
}

type PlaybackLatencySnapshot struct {
	Samples  []PlaybackLatencySample `json:"samples"`
	Total    int                     `json:"total"`
	Complete int                     `json:"complete"`
	Stats    LatencyStats            `json:"stats"`
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return -1
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func statFor(values []int64) latencyStat {
	if len(values) == 0 {
		return latencyStat{Min: -1, P50: -1, P95: -1, Max: -1, Avg: -1}
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var sum int64
	for _, v := range values {
		sum += v
	}
	return latencyStat{
		Min: sorted[0],
		P50: percentile(sorted, 0.50),
		P95: percentile(sorted, 0.95),
		Max: sorted[len(sorted)-1],
		Avg: sum / int64(len(values)),
		N:   len(values),
	}
}

func (t *PlaybackLatencyTracker) Snapshot(limit int) PlaybackLatencySnapshot {
	if t == nil {
		return PlaybackLatencySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	snap := PlaybackLatencySnapshot{Total: len(t.samples)}
	var totalVals, prequeueVals, hlsVals, warmupVals, serveVals []int64
	for _, s := range t.samples {
		if s.Complete {
			snap.Complete++
		}
		// Aggregate each phase from whatever the sample actually measures. A
		// prequeue-only sample (native SDR playback — no HLS segment) still has a
		// real prequeueMs, and a session that reused a ready prequeue still has
		// real hlsCreate/ffmpegWarmup/serveWait/total. Only the fully-correlated
		// ones count toward snap.Complete. Gating the stats on Complete alone is
		// what left every p50/p95 chip at -1ms whenever the bench was resolving
		// SDR releases.
		if s.TotalMs >= 0 {
			totalVals = append(totalVals, s.TotalMs)
		}
		if s.PrequeueMs >= 0 {
			prequeueVals = append(prequeueVals, s.PrequeueMs)
		}
		if s.HLSCreateMs >= 0 {
			hlsVals = append(hlsVals, s.HLSCreateMs)
		}
		if s.FFmpegWarmupMs >= 0 {
			warmupVals = append(warmupVals, s.FFmpegWarmupMs)
		}
		if s.ServeWaitMs >= 0 {
			serveVals = append(serveVals, s.ServeWaitMs)
		}
	}
	snap.Stats = LatencyStats{
		TotalMs:        statFor(totalVals),
		PrequeueMs:     statFor(prequeueVals),
		HLSCreateMs:    statFor(hlsVals),
		FFmpegWarmupMs: statFor(warmupVals),
		ServeWaitMs:    statFor(serveVals),
	}

	// Newest-first window.
	out := t.samples
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	rev := make([]PlaybackLatencySample, len(out))
	for i := range out {
		rev[len(out)-1-i] = out[i]
	}
	snap.Samples = rev
	return snap
}

// ---------------------------------------------------------------------------
// Admin surface: JSON endpoint + cache flush (cold-test support) + mini page.
// ---------------------------------------------------------------------------

// LatencyWindowResponse is the JSON body of the latency-window endpoint
// (ServePlaybackLatencyJSON). Field names mirror what the page's JS reads.
type LatencyWindowResponse struct {
	Samples  []PlaybackLatencySample `json:"samples"`
	Total    int                     `json:"total"`
	Complete int                     `json:"complete"`
	Stats    LatencyStats            `json:"stats"`
}

// LatencyClearResponse is the JSON body of the clear-samples endpoint.
type LatencyClearResponse struct {
	Cleared bool `json:"cleared"`
}

// PlaybackLatencyAdmin exposes the latency window to the admin surface. The
// tracker is populated passively by the playback path; this type only serves
// the samples (page + JSON) and the sample-window clear.
type PlaybackLatencyAdmin struct {
	tracker *PlaybackLatencyTracker
}

func NewPlaybackLatencyAdmin(tracker *PlaybackLatencyTracker) *PlaybackLatencyAdmin {
	return &PlaybackLatencyAdmin{tracker: tracker}
}

// ServePlaybackLatencyJSON renders the latest samples + aggregate stats.
func (a *PlaybackLatencyAdmin) ServePlaybackLatencyJSON(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &limit); err != nil || limit <= 0 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LatencyWindowResponse(a.tracker.Snapshot(limit)))
}

// ServeLatencyPage renders a small self-contained admin page for watching the
// click→first-frame numbers.
func (a *PlaybackLatencyAdmin) ServeLatencyPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, latencyPageHTML)
}

// ClearLatencySamples drops the sample window.
func (a *PlaybackLatencyAdmin) ClearLatencySamples(w http.ResponseWriter, r *http.Request) {
	if a.tracker != nil {
		a.tracker.ClearSamples()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LatencyClearResponse{Cleared: true})
}

const latencyPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Playback Latency</title>
<style>
  body { font-family: ui-monospace, Menlo, monospace; font-size: 13px; background: #111; color: #ddd; margin: 24px; }
  h1 { font-size: 18px; color: #fff; }
  .chips { display: flex; gap: 12px; flex-wrap: wrap; margin: 12px 0 18px; }
  .chip { background: #1d2733; border: 1px solid #2e3c4d; border-radius: 8px; padding: 8px 14px; }
  .chip b { color: #7fd4ff; }
  table { border-collapse: collapse; width: 100%; }
  th, td { text-align: right; padding: 5px 10px; border-bottom: 1px solid #26303b; }
  th { color: #8aa0b4; font-weight: 600; position: sticky; top: 0; background: #111; }
  td.l, th.l { text-align: left; }
  .muted { color: #8aa0b4; }
  .good { color: #6fdc8c; } .slow { color: #ffc76f; } .bad { color: #ff7b7b; }
  button { background: #27445f; color: #fff; border: none; border-radius: 6px; padding: 8px 14px; cursor: pointer; }
  button.danger { background: #5f2e2e; }
  .row { display: flex; gap: 10px; align-items: center; margin-bottom: 10px; }
  .note { color: #8aa0b4; max-width: 1000px; line-height: 1.5; }
  .cands { margin-top: 3px; font-size: 11px; line-height: 1.6; color: #8aa0b4; }
  .cand { display: inline-block; margin-right: 6px; padding: 1px 6px; border-radius: 4px;
    background: #1d2733; border: 1px solid #2e3c4d; }
  .cand.probe_rejected, .cand.articles_unavailable { color: #ff9d9d; border-color: #5f2e2e; }
  .cand.adopted { color: #6fdc8c; border-color: #1f4d33; }
  .cand.superseded { color: #8aa0b4; }
</style>
</head>
<body>
<h1>Playback Latency — click → first frame</h1>
<div class="note">
  Measures the server-side path: t0 = prequeue request (click), t1 = prequeue ready,
  t2 = HLS session created, t3 = first segment on disk, t4 = first segment response.
  Native (non-HLS) clients stop at t1 (see PREQUEUE_LATENCY in server logs).
</div>
<div class="row">
  <button class="danger" onclick="clearSamples()">Clear samples</button>
</div>
<div class="chips" id="chips"></div>
<table>
  <thead><tr>
    <th class="l">#</th><th class="l">Time</th><th class="l">Title</th><th class="l">Service</th>
    <th>Total ms</th><th>Prequeue ms</th><th>HLS create ms</th><th>FFmpeg warmup ms</th><th>Serve wait ms</th><th>Complete</th>
  </tr></thead>
  <tbody id="rows"></tbody>
</table>

<script>
async function api(path, opts) {
  const res = await fetch(path, opts);
  if (!res.ok) throw new Error(path + " -> " + res.status);
  return res.json();
}
function cls(ms) { if (ms < 0) return "muted"; if (ms < 5000) return "good"; if (ms < 15000) return "slow"; return "bad"; }
function fmtMs(ms) { return ms < 0 ? "–" : String(ms); }
function fmtTime(t) {
  if (!t) return "–";
  const d = new Date(t);
  return d.toLocaleTimeString() + "." + String(d.getMilliseconds()).padStart(3, "0");
}
async function refresh() {
  try {
    const snap = await api("/admin/api/latency?limit=60");
    const s = snap.stats;
    const chipData = [
      ["samples", snap.total], ["complete", snap.complete],
      ["total p50", s.totalMs.p50Ms + "ms"], ["total p95", s.totalMs.p95Ms + "ms"],
      ["total max", s.totalMs.maxMs + "ms"],
      ["prequeue p50", s.prequeueMs.p50Ms + "ms"], ["ffmpeg warmup p50", s.ffmpegWarmupMs.p50Ms + "ms"],
    ];
    document.getElementById("chips").innerHTML = chipData.map(function (c) {
      return '<div class="chip">' + c[0] + ": <b>" + c[1] + "</b></div>";
    }).join("");
    var rows = snap.samples.map(function (x) {
      var note = (x.notes || []).join("; ");
      var title = x.titleName || "–";
      var prov = x.serviceProvider ? " (" + escapeHtml(x.serviceProvider) + ")" : "";
      var when = fmtTime(x.firstSegmentSentAt || x.clientRequestedAt);
      var cands = (x.candidates || []).map(function (c) {
        return '<span class="cand ' + c.outcome + '" title="' + escapeHtml(c.releaseName || "") + '">' +
          '#' + c.index + '·' + c.outcome + '·' + fmtMs(c.durationMs) + 'ms</span>';
      }).join("");
      return '<tr>' +
        '<td class="l muted">' + x.id + '</td>' +
        '<td class="l muted" title="' + note + '">' + when + '</td>' +
        '<td class="l">' + escapeHtml(title) + (cands ? '<div class="cands">' + cands + '</div>' : '') + '</td>' +
        '<td class="l muted">' + (x.serviceType || "–") + prov + '</td>' +
        '<td class="' + cls(x.totalMs) + '">' + fmtMs(x.totalMs) + '</td>' +
        '<td class="' + cls(x.prequeueMs) + '">' + fmtMs(x.prequeueMs) + '</td>' +
        '<td class="' + cls(x.hlsCreateMs) + '">' + fmtMs(x.hlsCreateMs) + '</td>' +
        '<td class="' + cls(x.ffmpegWarmupMs) + '">' + fmtMs(x.ffmpegWarmupMs) + '</td>' +
        '<td class="' + cls(x.serveWaitMs) + '">' + fmtMs(x.serveWaitMs) + '</td>' +
        '<td>' + (x.complete ? "✅" : "–") + '</td>' +
      '</tr>';
    }).join("");
    document.getElementById("rows").innerHTML = rows;
  } catch (e) {
    document.getElementById("rows").innerHTML = '<tr><td colspan="10" class="muted">' + e + '</td></tr>';
  }
}
function escapeHtml(s) { const d = document.createElement("div"); d.textContent = s; return d.innerHTML; }
async function clearSamples() {
  try { await api("/admin/api/latency/clear", { method: "POST" }); refresh(); }
  catch (e) { alert("clear failed: " + e); }
}
setInterval(refresh, 2500);
refresh();
</script>
</body>
</html>
`
