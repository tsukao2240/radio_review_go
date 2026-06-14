package recordingmeta

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/recordingfile"
)

func FinalizeAAC(ctx context.Context, info *model.RecordingInfo, inputAACPath string) error {
	if info == nil || info.FilePath == "" || inputAACPath == "" {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fallbackPath := recordingfile.FallbackAACPath(info.FilePath)
		if err := os.Rename(inputAACPath, fallbackPath); err != nil {
			return fmt.Errorf("fallback aac rename failed: %w", err)
		}
		info.FilePath = fallbackPath
		return nil
	}
	tmp := info.FilePath + ".tagging.tmp.m4a"
	args := []string{
		"-y",
		"-f", "aac",
		"-i", inputAACPath,
		"-c", "copy",
		"-metadata", "title=" + info.ProgramName,
		"-metadata", "artist=" + info.StationID,
		"-metadata", "album=RadioProgram Review",
	}
	if date := recordingDate(info.StartTime); date != "" {
		args = append(args, "-metadata", "date="+date)
	}
	args = append(args, tmp)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...) //nolint:gosec // args are not evaluated by a shell.
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ffmpeg m4a remux failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmp, info.FilePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace m4a recording: %w", err)
	}
	_ = os.Remove(inputAACPath)
	return nil
}

func recordingDate(startTime string) string {
	if len(startTime) < 8 {
		return ""
	}
	for _, r := range startTime[:8] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return startTime[:4] + "-" + startTime[4:6] + "-" + startTime[6:8]
}
