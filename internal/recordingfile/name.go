package recordingfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidFileNameChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

func NewPath(storagePath, recordingID, startTime, stationID, programName string) string {
	base := BaseName(startTime, stationID, programName)
	path := filepath.Join(storagePath, base)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	suffix := shortID(recordingID)
	return filepath.Join(storagePath, fmt.Sprintf("%s_%s%s", stem, suffix, ext))
}

func BaseName(startTime, stationID, programName string) string {
	prefix := startPrefix(startTime)
	station := sanitize(stationID)
	title := sanitize(programName)
	if station == "" {
		station = "UNKNOWN"
	}
	if title == "" {
		title = "recording"
	}
	if prefix == "" {
		prefix = time.Now().Format("2006-01-02_1504")
	}
	return fmt.Sprintf("%s_%s_%s.m4a", prefix, station, title)
}

func DisplayName(infoFilePath, startTime, stationID, programName string) string {
	if infoFilePath != "" {
		return filepath.Base(infoFilePath)
	}
	return BaseName(startTime, stationID, programName)
}

func ContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	default:
		return "application/octet-stream"
	}
}

func IsSupportedRecordingPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4a", ".aac":
		return true
	default:
		return false
	}
}

func TempAACPath(finalPath string) string {
	return finalPath + ".download.aac"
}

func FallbackAACPath(finalPath string) string {
	ext := filepath.Ext(finalPath)
	base := strings.TrimSuffix(finalPath, ext) + ".aac"
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	stem := strings.TrimSuffix(base, ".aac")
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d.aac", stem, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func startPrefix(startTime string) string {
	if len(startTime) < 12 {
		return ""
	}
	raw := startTime
	if len(raw) == 12 {
		raw += "00"
	}
	if len(raw) < 14 {
		return ""
	}
	for _, r := range raw[:14] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	base, err := time.ParseInLocation("20060102", raw[:8], time.Local)
	if err != nil {
		return ""
	}
	hour := atoi(raw[8:10])
	minute := atoi(raw[10:12])
	at := base.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	return at.Format("2006-01-02_1504")
}

func sanitize(s string) string {
	s = invalidFileNameChars.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= 80 {
		return s
	}
	return string([]rune(s)[:80])
}

func shortID(recordingID string) string {
	if len(recordingID) <= 8 {
		return recordingID
	}
	return recordingID[len(recordingID)-8:]
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
