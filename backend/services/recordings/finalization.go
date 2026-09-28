package recordings

import (
	"fmt"
	"log"
	"os"

	"novastream/models"
)

// repairRecordingOutput repairs saved media independently of capture status.
// A successful repair does not erase the reason a capture was incomplete.
func (s *Service) repairRecordingOutput(recording *models.Recording, captureError string) string {
	info, err := os.Stat(recording.OutputPath)
	if err != nil || info.Size() == 0 {
		return captureError
	}
	recording.OutputSizeBytes = info.Size()
	if err := s.remuxRecording(recording.OutputPath); err != nil {
		log.Printf("[recordings] repair recording %s failed: %v", recording.ID, err)
		repairError := fmt.Sprintf("repair recording timestamps: %v", err)
		if captureError == "" {
			return truncateRecordingError(repairError)
		}
		return truncateRecordingError(captureError + "; " + repairError)
	}
	if info, err := os.Stat(recording.OutputPath); err == nil {
		recording.OutputSizeBytes = info.Size()
	}
	return captureError
}
