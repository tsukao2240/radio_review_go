package radiko

import (
	"net/url"
	"testing"
)

func TestResolveURL(t *testing.T) {
	t.Run("absolute URL is returned as-is", func(t *testing.T) {
		base, err := url.Parse("https://example.com/path/playlist.m3u8")
		if err != nil {
			t.Fatalf("url.Parse base: %v", err)
		}

		rawURL := "https://cdn.example.com/segment/001.ts"
		got, err := resolveURL(base, rawURL)
		if err != nil {
			t.Fatalf("resolveURL error: %v", err)
		}
		if got != rawURL {
			t.Errorf("got %q, want %q", got, rawURL)
		}
	})

	t.Run("relative URL is resolved against base", func(t *testing.T) {
		base, err := url.Parse("https://example.com/path/playlist.m3u8")
		if err != nil {
			t.Fatalf("url.Parse base: %v", err)
		}

		rawURL := "segment/001.ts"
		got, err := resolveURL(base, rawURL)
		if err != nil {
			t.Fatalf("resolveURL error: %v", err)
		}
		want := "https://example.com/path/segment/001.ts"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("path-only URL is resolved with base host", func(t *testing.T) {
		base, err := url.Parse("https://example.com/path/playlist.m3u8")
		if err != nil {
			t.Fatalf("url.Parse base: %v", err)
		}

		rawURL := "/segments/002.ts"
		got, err := resolveURL(base, rawURL)
		if err != nil {
			t.Fatalf("resolveURL error: %v", err)
		}
		want := "https://example.com/segments/002.ts"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("URL with query parameters is resolved correctly", func(t *testing.T) {
		base, err := url.Parse("https://radiko.jp/v2/api/ts/playlist.m3u8?station_id=TBS&ft=202401010000&to=202401020000")
		if err != nil {
			t.Fatalf("url.Parse base: %v", err)
		}

		rawURL := "https://radiko.jp/v2/api/ts/chunklist.m3u8"
		got, err := resolveURL(base, rawURL)
		if err != nil {
			t.Fatalf("resolveURL error: %v", err)
		}
		if got != rawURL {
			t.Errorf("got %q, want %q", got, rawURL)
		}
	})

	t.Run("empty raw URL returns base URL without path", func(t *testing.T) {
		base, err := url.Parse("https://example.com/path/playlist.m3u8")
		if err != nil {
			t.Fatalf("url.Parse base: %v", err)
		}

		rawURL := ""
		got, err := resolveURL(base, rawURL)
		if err != nil {
			t.Fatalf("resolveURL error: %v", err)
		}
		// empty string resolves to the base URL itself
		want := "https://example.com/path/playlist.m3u8"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
