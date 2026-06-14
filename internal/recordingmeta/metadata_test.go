package recordingmeta

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func TestFinalizeAACFallsBackWhenFFmpegUnavailable(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "recording.m4a.download.aac")
	finalPath := filepath.Join(dir, "recording.m4a")
	if err := os.WriteFile(inputPath, []byte("aac"), 0600); err != nil {
		t.Fatal(err)
	}
	info := &model.RecordingInfo{
		FilePath:    finalPath,
		ProgramName: "Show",
		StationID:   "TBS",
		StartTime:   "20260605010000",
	}
	err := FinalizeAAC(context.Background(), info, inputPath)
	if err != nil {
		t.Fatalf("FinalizeAAC: %v", err)
	}
	if info.FilePath != filepath.Join(dir, "recording.aac") {
		t.Fatalf("FilePath = %q", info.FilePath)
	}
	got, err := os.ReadFile(info.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "aac" {
		t.Fatalf("file changed: %q", got)
	}
	if _, err := os.Stat(inputPath); !os.IsNotExist(err) {
		t.Fatalf("input file still exists or unexpected stat error: %v", err)
	}
}

func TestRecordingDate(t *testing.T) {
	if got := recordingDate("20260605010000"); got != "2026-06-05" {
		t.Fatalf("recordingDate = %q", got)
	}
}
