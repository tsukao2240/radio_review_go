package recordingmeta

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func TagAAC(ctx context.Context, info *model.RecordingInfo) error {
	if info == nil || info.FilePath == "" {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}
	tmp := info.FilePath + ".tagging.tmp"
	args := []string{
		"-y",
		"-i", info.FilePath,
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
		return fmt.Errorf("ffmpeg metadata copy failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmp, info.FilePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace tagged recording: %w", err)
	}
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
