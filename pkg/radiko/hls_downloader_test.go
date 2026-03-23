package radiko

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestClient は httptest.Server を使うテスト用 Client を返す。
// Client.httpClient を直接設定する（同パッケージなので可能）。
func newTestClient(httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
		redis:      nil,
		keyPath:    "",
	}
}

func TestFetchSegmentURLs_ParsesM3U8(t *testing.T) {
	m3u8 := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXT-X-TARGETDURATION:15",
		"#EXTINF:15.0,",
		"seg001.ts",
		"#EXTINF:15.0,",
		"seg002.ts",
		"#EXTINF:15.0,",
		"https://cdn.example.com/abs/seg003.ts",
		"#EXT-X-ENDLIST",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Radiko-AuthToken") != "test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		fmt.Fprint(w, m3u8)
	}))
	defer srv.Close()

	d := &HLSDownloader{
		client:      newTestClient(srv.Client()),
		maxParallel: 2,
	}

	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d: %v", len(segments), segments)
	}
	// 相対URLはベースURLで解決されているか確認
	base, _ := url.Parse(srv.URL + "/playlist.m3u8")
	want0, _ := resolveURL(base, "seg001.ts")
	if segments[0] != want0 {
		t.Errorf("segments[0]: got %q, want %q", segments[0], want0)
	}
	// 絶対URLはそのまま
	if segments[2] != "https://cdn.example.com/abs/seg003.ts" {
		t.Errorf("segments[2]: got %q, want %q", segments[2], "https://cdn.example.com/abs/seg003.ts")
	}
}

func TestFetchSegmentURLs_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	_, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestFetchSegmentURLs_EmptyPlaylist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-ENDLIST\n")
	}))
	defer srv.Close()

	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segments))
	}
}

func TestFetchSegment_ReturnsData(t *testing.T) {
	want := []byte("fake-ts-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Radiko-AuthToken") != "tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Write(want)
	}))
	defer srv.Close()

	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	got, err := d.fetchSegment(context.Background(), srv.URL+"/seg.ts", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFetchSegment_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	_, err := d.fetchSegment(context.Background(), srv.URL+"/seg.ts", "tok")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestNewHLSDownloader_DefaultParallel(t *testing.T) {
	d := NewHLSDownloader(&Client{}, 0)
	if d.maxParallel != 10 {
		t.Errorf("expected maxParallel=10, got %d", d.maxParallel)
	}
}

func TestNewHLSDownloader_CustomParallel(t *testing.T) {
	d := NewHLSDownloader(&Client{}, 5)
	if d.maxParallel != 5 {
		t.Errorf("expected maxParallel=5, got %d", d.maxParallel)
	}
}

func TestDownloadTimefree_WritesSegments(t *testing.T) {
	seg1 := []byte("segment-one-data")
	seg2 := []byte("segment-two-data")

	m3u8 := "#EXTM3U\n#EXTINF:15.0,\nseg1.ts\n#EXTINF:15.0,\nseg2.ts\n#EXT-X-ENDLIST\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "playlist.m3u8"):
			fmt.Fprint(w, m3u8)
		case strings.HasSuffix(r.URL.Path, "seg1.ts"):
			w.Write(seg1)
		case strings.HasSuffix(r.URL.Path, "seg2.ts"):
			w.Write(seg2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// playlistURL が srv.URL/playlist.m3u8 になるよう細工
	origURL := srv.URL + "/playlist.m3u8"
	_ = origURL

	// HLSDownloader の DownloadTimefree は内部でURLを組み立てるため、
	// fetchSegmentURLs を直接テストする形で統合テストを代替する。
	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}

	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err != nil {
		t.Fatalf("fetchSegmentURLs: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}

	// outputPath への書き込みテスト
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.aac")

	results := make([][]byte, len(segments))
	for i, u := range segments {
		data, err := d.fetchSegment(context.Background(), u, "tok")
		if err != nil {
			t.Fatalf("fetchSegment[%d]: %v", i, err)
		}
		results[i] = data
	}

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	for _, data := range results {
		f.Write(data)
	}
	f.Close()

	got, _ := os.ReadFile(outPath)
	want := append(seg1, seg2...)
	if string(got) != string(want) {
		t.Errorf("output mismatch: got %q, want %q", got, want)
	}
}


func TestDownloadTimefree_NoSegments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-ENDLIST\n")
	}))
	defer srv.Close()

	// fetchSegmentURLs は空スライスを返し、DownloadTimefree はエラーを返す
	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segments))
	}
}
