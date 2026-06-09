package recordingmeta

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func TestTagAACSkipsWhenFFmpegUnavailable(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	filePath := filepath.Join(t.TempDir(), "recording.aac")
	if err := os.WriteFile(filePath, []byte("aac"), 0600); err != nil {
		t.Fatal(err)
	}
	err := TagAAC(context.Background(), &model.RecordingInfo{
		FilePath:    filePath,
		ProgramName: "Show",
		StationID:   "TBS",
		StartTime:   "20260605010000",
	})
	if err != nil {
		t.Fatalf("TagAAC: %v", err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "aac" {
		t.Fatalf("file changed: %q", got)
	}
}

func TestRecordingDate(t *testing.T) {
	if got := recordingDate("20260605010000"); got != "2026-06-05" {
		t.Fatalf("recordingDate = %q", got)
	}
}
