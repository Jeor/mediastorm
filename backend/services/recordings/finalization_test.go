package recordings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"novastream/models"
)

func TestInterruptedRecordingRepairsPartialMedia(t *testing.T) {
	for _, repairFails := range []bool{false, true} {
		name := "success"
		if repairFails {
			name = "repair failure"
		}
		t.Run(name, func(t *testing.T) {
			recording := newTestRecording(time.Now().Add(time.Minute))
			recording.OutputPath = filepath.Join(t.TempDir(), "partial.ts")
			recording.Status = models.RecordingStatusRunning
			if err := os.WriteFile(recording.OutputPath, []byte("original"), 0o644); err != nil {
				t.Fatal(err)
			}
			svc := newTestService(newFakeRecordingRepo(recording), "", "")
			repairs := 0
			svc.remuxRecording = func(path string) error {
				repairs++
				if repairFails {
					return errors.New("disk full")
				}
				return os.WriteFile(path, []byte("repaired media"), 0o644)
			}
			ended := time.Now().UTC()
			svc.handleInterruptedRecording(recording.ID, ended)
			latest, err := svc.Get(recording.ID)
			if err != nil {
				t.Fatal(err)
			}
			if repairs != 1 || latest.Status != models.RecordingStatusFailed || !strings.Contains(latest.Error, "interrupted before scheduled stop") {
				t.Fatalf("repairs=%d, recording=%+v", repairs, latest)
			}
			if strings.Contains(latest.Error, "disk full") != repairFails {
				t.Fatalf("repair error not preserved: %q", latest.Error)
			}
			info, err := os.Stat(recording.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			if latest.OutputSizeBytes != info.Size() || latest.ActualEndAt == nil || !latest.ActualEndAt.Equal(ended) {
				t.Fatalf("incorrect finalized metadata: %+v", latest)
			}
		})
	}
}

func TestRepairRecordingSkipsMissingOrEmptyMedia(t *testing.T) {
	for _, empty := range []bool{false, true} {
		recording := models.Recording{OutputPath: filepath.Join(t.TempDir(), "empty.ts")}
		if empty {
			if err := os.WriteFile(recording.OutputPath, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		svc := newTestService(nil, "", "")
		svc.remuxRecording = func(string) error {
			t.Fatal("remux without media")
			return nil
		}
		if got := svc.repairRecordingOutput(&recording, "source failed"); got != "source failed" {
			t.Fatalf("capture error = %q", got)
		}
	}
}

func TestFailedRemuxPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recording.ts")
	if err := os.WriteFile(path, []byte("original media"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := writeFakeFFmpeg(t, "#!/bin/sh\nfor arg do output=\"$arg\"; done\nprintf partial > \"$output\"\nexit 1\n")
	if err := remuxRecordingTimestamps(script, path); err == nil {
		t.Fatal("expected remux failure")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original media" {
		t.Fatalf("original changed: %q, %v", data, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary output not cleaned up: %v, %v", files, err)
	}
}
