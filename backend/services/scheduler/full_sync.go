package scheduler

import (
	"fmt"

	"novastream/config"
)

// prepareFullSync overrides only the execution copy. Saved task settings and
// cursors remain intact until the run records its result.
func prepareFullSync(task config.ScheduledTask) (config.ScheduledTask, error) {
	switch task.Type {
	case config.ScheduledTaskTypePlexWatchlistSync,
		config.ScheduledTaskTypeTraktListSync,
		config.ScheduledTaskTypeTraktHistorySync,
		config.ScheduledTaskTypeSimklHistorySync,
		config.ScheduledTaskTypeScrobHistorySync,
		config.ScheduledTaskTypePlexHistorySync,
		config.ScheduledTaskTypeJellyfinFavoritesSync,
		config.ScheduledTaskTypeJellyfinHistorySync,
		config.ScheduledTaskTypeMDBListWatchlistSync,
		config.ScheduledTaskTypeMDBListHistorySync:
	default:
		return task, fmt.Errorf("full sync is only available for sync tasks")
	}
	task.LastRunAt = nil
	copied := make(map[string]string, len(task.Config)+2)
	for key, value := range task.Config {
		copied[key] = value
	}
	task.Config = copied
	task.Config["fullSync"] = "true"
	task.Config["fullExport"] = "true"
	delete(task.Config, "lastSimklActivityAt")
	return task, nil
}
