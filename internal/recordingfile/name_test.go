package recordingfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseName(t *testing.T) {
	got := BaseName("20260605010000", "TBS", "マヂカルラブリーのオールナイトニッポン0")
	want := "2026-06-05_0100_TBS_マヂカルラブリーのオールナイトニッポン0.m4a"
	if got != want {
		t.Fatalf("BaseName = %q, want %q", got, want)
	}
}

func TestBaseNameAfter24Hour(t *testing.T) {
	got := BaseName("20260605250000", "TBS", "深夜番組")
	want := "2026-06-06_0100_TBS_深夜番組.m4a"
	if got != want {
		t.Fatalf("BaseName = %q, want %q", got, want)
	}
}

func TestBaseNameSanitizesForbiddenCharacters(t *testing.T) {
	got := BaseName("20260605010000", "TB/S", `bad:/\*?"<>|title`)
	if strings.ContainsAny(got, `\/:*?"<>|`) {
		t.Fatalf("BaseName contains forbidden characters: %q", got)
	}
	if !strings.Contains(got, "TBS_badtitle.m4a") {
		t.Fatalf("BaseName = %q, want sanitized station and title", got)
	}
}

func TestNewPathAddsShortIDOnDuplicate(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "2026-06-05_0100_TBS_Show.m4a")
	if err := os.WriteFile(first, []byte("exists"), 0600); err != nil {
		t.Fatal(err)
	}
	got := NewPath(dir, "1234567890abcdef", "20260605010000", "TBS", "Show")
	want := filepath.Join(dir, "2026-06-05_0100_TBS_Show_90abcdef.m4a")
	if got != want {
		t.Fatalf("NewPath = %q, want %q", got, want)
	}
}

func TestContentType(t *testing.T) {
	if got := ContentType("recording.m4a"); got != "audio/mp4" {
		t.Fatalf("m4a ContentType = %q", got)
	}
	if got := ContentType("recording.aac"); got != "audio/aac" {
		t.Fatalf("aac ContentType = %q", got)
	}
}

func TestFallbackAACPathAvoidsExistingFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "show.aac")
	if err := os.WriteFile(existing, []byte("exists"), 0600); err != nil {
		t.Fatal(err)
	}
	got := FallbackAACPath(filepath.Join(dir, "show.m4a"))
	want := filepath.Join(dir, "show_1.aac")
	if got != want {
		t.Fatalf("FallbackAACPath = %q, want %q", got, want)
	}
}
