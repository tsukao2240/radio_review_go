package recordingmeta

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func TestFinalizeAACUsesAACDemuxer(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(dir, "ffmpeg.args")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$FFMPEG_ARGS_PATH"
out=""
for arg do
	out="$arg"
done
printf 'm4a' > "$out"
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("FFMPEG_ARGS_PATH", argsPath)

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

	if err := FinalizeAAC(context.Background(), info, inputPath); err != nil {
		t.Fatalf("FinalizeAAC: %v", err)
	}

	rawArgs, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(rawArgs)), "\n")
	if len(args) < 3 || args[1] != "-f" || args[2] != "aac" {
		t.Fatalf("ffmpeg args = %#v, want -f aac before input", args)
	}
	if _, err := os.Stat(inputPath); !os.IsNotExist(err) {
		t.Fatalf("input file still exists or unexpected stat error: %v", err)
	}
	if got, err := os.ReadFile(finalPath); err != nil || string(got) != "m4a" {
		t.Fatalf("final file = %q, %v", got, err)
	}
}

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

func TestFinalizeAACWithRealFFmpeg(t *testing.T) {
	if os.Getenv("RECORDINGMETA_REAL_FFMPEG") != "1" {
		t.Skip("set RECORDINGMETA_REAL_FFMPEG=1 to run real ffmpeg integration test")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}

	dir := os.Getenv("RECORDINGMETA_REAL_FFMPEG_DIR")
	if dir == "" {
		dir = t.TempDir()
	} else if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "recordingmeta-real-" + time.Now().Format("20060102150405")
	inputPath := filepath.Join(dir, name+".m4a.download.aac")
	finalPath := filepath.Join(dir, name+".m4a")
	createArgs := []string{
		"-y",
		"-f", "lavfi",
		"-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
		"-t", "60",
		"-c:a", "aac",
		"-f", "adts",
		inputPath,
	}
	if output, err := exec.Command("ffmpeg", createArgs...).CombinedOutput(); err != nil {
		t.Fatalf("create aac: %v: %s", err, strings.TrimSpace(string(output)))
	}
	info := &model.RecordingInfo{
		FilePath:    finalPath,
		ProgramName: "Real FFmpeg Show",
		StationID:   "TBS",
		StartTime:   "20260605010000",
	}

	if err := FinalizeAAC(context.Background(), info, inputPath); err != nil {
		t.Fatalf("FinalizeAAC: %v", err)
	}

	if _, err := os.Stat(inputPath); !os.IsNotExist(err) {
		t.Fatalf("input file still exists or unexpected stat error: %v", err)
	}
	probeArgs := []string{
		"-v", "error",
		"-show_entries", "format_tags=title,artist,album,date",
		"-of", "default=noprint_wrappers=1:nokey=0",
		finalPath,
	}
	output, err := exec.Command("ffprobe", probeArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe: %v: %s", err, strings.TrimSpace(string(output)))
	}
	got := string(output)
	t.Logf("final file: %s", finalPath)
	t.Logf("ffprobe output:\n%s", got)
	for _, want := range []string{
		"TAG:title=Real FFmpeg Show",
		"TAG:artist=TBS",
		"TAG:album=RadioProgram Review",
		"TAG:date=2026-06-05",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ffprobe output missing %q:\n%s", want, got)
		}
	}
}

func TestRecordingDate(t *testing.T) {
	if got := recordingDate("20260605010000"); got != "2026-06-05" {
		t.Fatalf("recordingDate = %q", got)
	}
}
