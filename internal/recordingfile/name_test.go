package recordingfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseName(t *testing.T) {
	got := BaseName("20260605010000", "TBS", "マヂカルラブリーのオールナイトニッポン0")
	want := "TBS_マヂカルラブリーのオールナイトニッポン0_202606042500.m4a"
	if got != want {
		t.Fatalf("BaseName = %q, want %q", got, want)
	}
}

func TestBaseNameAfter24Hour(t *testing.T) {
	got := BaseName("20260605250000", "TBS", "深夜番組")
	want := "TBS_深夜番組_202606052500.m4a"
	if got != want {
		t.Fatalf("BaseName = %q, want %q", got, want)
	}
}

func TestBaseNameSanitizesForbiddenCharacters(t *testing.T) {
	got := BaseName("20260605010000", "TB/S", `bad:/\*?"<>|title`)
	if strings.ContainsAny(got, `\/:*?"<>|`) {
		t.Fatalf("BaseName contains forbidden characters: %q", got)
	}
	if !strings.Contains(got, "TBS_badtitle_202606042500.m4a") {
		t.Fatalf("BaseName = %q, want sanitized station and title", got)
	}
}

func TestNewPathAddsShortIDOnDuplicate(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "TBS_Show_202606042500.m4a")
	if err := os.WriteFile(first, []byte("exists"), 0600); err != nil {
		t.Fatal(err)
	}
	got := NewPath(dir, "1234567890abcdef", "20260605010000", "TBS", "Show")
	want := filepath.Join(dir, "TBS_Show_202606042500_90abcdef.m4a")
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

func TestBroadcastFileName(t *testing.T) {
	for _, tc := range []struct{ start, want string }{
		{"20260912030000", "LFR_番組名_202609112700.m4a"},
		{"20260911270000", "LFR_番組名_202609112700.m4a"},
		{"20260101000000", "LFR_番組名_202512312400.m4a"},
		{"20260912050000", "LFR_番組名_202609120500.m4a"},
		{"20260911230000", "LFR_番組名_202609112300.m4a"},
	} {
		if got := BaseName(tc.start, "LFR", "番組名"); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.start, got, tc.want)
		}
	}
}
