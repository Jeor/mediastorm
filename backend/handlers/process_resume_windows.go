//go:build windows

package handlers

// Windows has no SIGCONT equivalent. MediaStorm's active throttling path uses
// pipe backpressure rather than suspending FFmpeg, so there is nothing to do.
func resumeProcess(_ int) error {
	return nil
}
