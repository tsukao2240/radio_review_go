package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/recordingfile"
)

func TestRenameRecordings(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "1717555200000000000_Jazz Show.aac")
	if err := os.WriteFile(oldPath, []byte("audio"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	info := model.RecordingInfo{
		RecordingID: "rec123456789",
		StationID:   "TBS",
		ProgramName: "Jazz Show",
		StartTime:   "20260605010000",
		FilePath:    oldPath,
	}
	newPath := recordingfile.NewPath(dir, info.RecordingID, info.StartTime, info.StationID, info.ProgramName)
	data, _ := json.Marshal(info)
	if err := rdb.Set(context.Background(), "recording_rec123456789", string(data), 0).Err(); err != nil {
		t.Fatalf("redis Set: %v", err)
	}

	result, err := renameRecordings(context.Background(), rdb, dir, false)
	if err != nil {
		t.Fatalf("renameRecordings: %v", err)
	}
	if result.renamed != 1 || result.failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new file missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or unexpected stat error: %v", err)
	}
	raw, err := rdb.Get(context.Background(), "recording_rec123456789").Result()
	if err != nil {
		t.Fatalf("redis Get: %v", err)
	}
	var updated model.RecordingInfo
	if err := json.Unmarshal([]byte(raw), &updated); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if updated.FilePath != newPath {
		t.Fatalf("FilePath = %q, want %q", updated.FilePath, newPath)
	}
}

func TestRenameRecordingsDryRun(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "1717555200000000000_Jazz Show.aac")
	if err := os.WriteFile(oldPath, []byte("audio"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	info := model.RecordingInfo{
		RecordingID: "rec123456789",
		StationID:   "TBS",
		ProgramName: "Jazz Show",
		StartTime:   "20260605010000",
		FilePath:    oldPath,
	}
	data, _ := json.Marshal(info)
	if err := rdb.Set(context.Background(), "recording_rec123456789", string(data), 0).Err(); err != nil {
		t.Fatalf("redis Set: %v", err)
	}

	result, err := renameRecordings(context.Background(), rdb, dir, true)
	if err != nil {
		t.Fatalf("renameRecordings: %v", err)
	}
	if result.renamed != 1 || result.failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old file missing after dry-run: %v", err)
	}
}

func TestIsLegacyRecordingName(t *testing.T) {
	cases := map[string]bool{
		"1717555200000000000_Jazz Show.aac": true,
		"2026-06-05_0100_TBS_Jazz Show.aac": false,
		"1717555200000000000_Jazz Show.mp3": false,
		"Jazz Show.aac":                     false,
	}
	for name, want := range cases {
		if got := isLegacyRecordingName(name); got != want {
			t.Fatalf("isLegacyRecordingName(%q) = %t, want %t", name, got, want)
		}
	}
}
