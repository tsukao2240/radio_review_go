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

func TestFetchSegmentURLs_ParsesMasterPlaylist(t *testing.T) {
	mediaM3U8 := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXTINF:15.0,",
		"seg001.aac",
		"#EXTINF:15.0,",
		"https://cdn.example.com/seg002.aac",
		"#EXT-X-ENDLIST",
	}, "\n")
	masterM3U8 := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:BANDWIDTH=48000,CODECS=\"mp4a.40.2\"",
		"medialist?session=abc",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Radiko-AuthToken") != "tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/playlist.m3u8":
			fmt.Fprint(w, masterM3U8)
		case "/medialist":
			fmt.Fprint(w, mediaM3U8)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}
	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err != nil {
		t.Fatalf("fetchSegmentURLs: %v", err)
	}
	want := []string{
		srv.URL + "/seg001.aac",
		"https://cdn.example.com/seg002.aac",
	}
	if len(segments) != len(want) {
		t.Fatalf("got %d segments %v, want %d", len(segments), segments, len(want))
	}
	for i := range want {
		if segments[i] != want[i] {
			t.Errorf("segments[%d] = %q, want %q", i, segments[i], want[i])
		}
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

func TestDownloadTimefree_FullFlow(t *testing.T) {
	seg1 := []byte("audio-segment-1")
	seg2 := []byte("audio-segment-2")

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

	// Override playlistURL by testing through fetchSegmentURLs + download
	d := &HLSDownloader{client: newTestClient(srv.Client()), maxParallel: 2}

	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.aac")

	// Test: fetch segments then download each
	segments, err := d.fetchSegmentURLs(context.Background(), srv.URL+"/playlist.m3u8", "tok")
	if err != nil {
		t.Fatalf("fetchSegmentURLs: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}

	// Write segments to file manually (same as DownloadTimefree logic)
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	for _, u := range segments {
		data, err := d.fetchSegment(context.Background(), u, "tok")
		if err != nil {
			f.Close()
			t.Fatalf("fetchSegment: %v", err)
		}
		f.Write(data)
	}
	f.Close()

	got, _ := os.ReadFile(outPath)
	want := append(seg1, seg2...)
	if string(got) != string(want) {
		t.Errorf("output mismatch: got %q, want %q", got, want)
	}
}

// roundTripFunc はカスタム http.RoundTripper。
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHLSDownloader_DownloadTimefree(t *testing.T) {
	seg1 := []byte("ts-data-1")
	seg2 := []byte("ts-data-2")
	streamXML := `<?xml version="1.0" encoding="UTF-8"?>
<urls>
  <url timefree="1" areafree="0">
    <playlist_create_url>https://radiko.jp/v3/timetable/playlist.m3u8</playlist_create_url>
  </url>
</urls>`

	var srvAddr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/v3/station/stream/pc_html5/TBS.xml"):
			fmt.Fprint(w, streamXML)
		case strings.Contains(r.URL.Path, "playlist.m3u8"):
			// Return absolute segment URLs using the server address
			base := "http://" + srvAddr
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:15.0,\n%s/seg1.ts\n#EXTINF:15.0,\n%s/seg2.ts\n#EXT-X-ENDLIST\n",
				base, base)
		case strings.HasSuffix(r.URL.Path, "seg1.ts"):
			w.Write(seg1)
		case strings.HasSuffix(r.URL.Path, "seg2.ts"):
			w.Write(seg2)
		default:
			http.NotFound(w, r)
		}
	}))
	srvAddr = srv.Listener.Addr().String()
	defer srv.Close()

	// Create a client that redirects all requests to srv
	testTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Rewrite host to srv
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})
	testHTTPClient := &http.Client{Transport: testTransport}

	d := &HLSDownloader{client: newTestClient(testHTTPClient), maxParallel: 2}

	dir := t.TempDir()
	outPath := filepath.Join(dir, "timefree.aac")

	err := d.DownloadTimefree(context.Background(), "test-token", "TBS", "20240101100000", "20240101110000", "JP13", outPath)
	if err != nil {
		t.Fatalf("DownloadTimefree: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := append(seg1, seg2...)
	if string(got) != string(want) {
		t.Errorf("output mismatch: got %q, want %q", got, want)
	}
}

func TestHLSDownloader_DownloadTimefree_EmptyPlaylist(t *testing.T) {
	streamXML := `<?xml version="1.0" encoding="UTF-8"?>
<urls>
  <url timefree="1" areafree="0">
    <playlist_create_url>https://radiko.jp/v3/timetable/playlist.m3u8</playlist_create_url>
  </url>
</urls>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v3/station/stream/pc_html5/TBS.xml") {
			fmt.Fprint(w, streamXML)
			return
		}
		if strings.Contains(r.URL.Path, "playlist.m3u8") {
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-ENDLIST\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	testTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})
	testHTTPClient := &http.Client{Transport: testTransport}

	d := &HLSDownloader{client: newTestClient(testHTTPClient), maxParallel: 2}

	dir := t.TempDir()
	outPath := filepath.Join(dir, "empty.aac")

	err := d.DownloadTimefree(context.Background(), "tok", "TBS", "20240101100000", "20240101110000", "JP13", outPath)
	if err == nil {
		t.Error("expected error for empty playlist, got nil")
	}
}

func TestBuildTimefreePlaylistURLs_NormalizesTwelveDigitTimes(t *testing.T) {
	streamXML := `<?xml version="1.0" encoding="UTF-8"?>
<urls>
  <url timefree="1" areafree="1">
    <playlist_create_url>https://radiko.jp/areafree.m3u8</playlist_create_url>
  </url>
  <url timefree="1" areafree="0">
    <playlist_create_url>https://radiko.jp/timefree.m3u8</playlist_create_url>
  </url>
</urls>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, streamXML)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})
	d := &HLSDownloader{client: newTestClient(&http.Client{Transport: transport}), maxParallel: 2}

	urls, err := d.buildTimefreePlaylistURLs(context.Background(), "TBS", "202401011000", "202401011010", "tok", "JP13")
	if err != nil {
		t.Fatalf("buildTimefreePlaylistURLs: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("got %d urls, want 2: %v", len(urls), urls)
	}
	parsed, err := url.Parse(urls[0])
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if parsed.Host != "radiko.jp" || parsed.Path != "/timefree.m3u8" {
		t.Fatalf("base URL = %s, want https://radiko.jp/timefree.m3u8", parsed.String())
	}
	q := parsed.Query()
	if q.Get("ft") != "20240101100000" || q.Get("to") != "20240101101000" || q.Get("seek") != "20240101100000" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
	if q.Get("start_at") != "20240101100000" || q.Get("end_at") != "20240101101000" || q.Get("l") != "300" || q.Get("type") != "b" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
}

func TestBuildTimefreePlaylistURLs_UsesAreaFreeURLAndTypeC(t *testing.T) {
	streamXML := `<?xml version="1.0" encoding="UTF-8"?>
<urls>
  <url timefree="1" areafree="0">
    <playlist_create_url>https://radiko.jp/timefree.m3u8</playlist_create_url>
  </url>
  <url timefree="1" areafree="1">
    <playlist_create_url>https://radiko.jp/areafree.m3u8</playlist_create_url>
  </url>
</urls>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, streamXML)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})
	d := &HLSDownloader{client: newTestClient(&http.Client{Transport: transport}), maxParallel: 2}

	urls, err := d.buildTimefreePlaylistURLs(context.Background(), "OBC", "202401011000", "202401011005", "tok", "JP27")
	if err != nil {
		t.Fatalf("buildTimefreePlaylistURLs: %v", err)
	}
	parsed, err := url.Parse(urls[0])
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if parsed.Path != "/areafree.m3u8" {
		t.Fatalf("path = %q, want /areafree.m3u8", parsed.Path)
	}
	if got := parsed.Query().Get("type"); got != "c" {
		t.Fatalf("type = %q, want c", got)
	}
}

func TestNormalizeRadikoTimestamp_NormalizesOverMidnightHours(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "24 hour twelve digits", input: "202606042400", want: "20260605000000"},
		{name: "25 hour fourteen digits", input: "20260604250000", want: "20260605010000"},
		{name: "26 hour", input: "20260604263000", want: "20260605023000"},
		{name: "27 hour", input: "20260604275930", want: "20260605035930"},
		{name: "28 hour", input: "20260604280000", want: "20260605040000"},
		{name: "29 hour", input: "20260604290000", want: "20260605050000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRadikoTimestamp(tt.input)
			if err != nil {
				t.Fatalf("normalizeRadikoTimestamp(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeRadikoTimestamp(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildTimefreePlaylistURLs_NormalizesOverMidnightEndTime(t *testing.T) {
	streamXML := `<?xml version="1.0" encoding="UTF-8"?>
<urls>
  <url timefree="1" areafree="0">
    <playlist_create_url>https://radiko.jp/timefree.m3u8</playlist_create_url>
  </url>
</urls>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, streamXML)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})
	d := &HLSDownloader{client: newTestClient(&http.Client{Transport: transport}), maxParallel: 2}

	urls, err := d.buildTimefreePlaylistURLs(context.Background(), "OBC", "20260604240000", "20260604250000", "tok", "JP27")
	if err != nil {
		t.Fatalf("buildTimefreePlaylistURLs: %v", err)
	}
	if len(urls) != 12 {
		t.Fatalf("got %d urls, want 12: %v", len(urls), urls)
	}
	first, err := url.Parse(urls[0])
	if err != nil {
		t.Fatalf("url.Parse first: %v", err)
	}
	last, err := url.Parse(urls[len(urls)-1])
	if err != nil {
		t.Fatalf("url.Parse last: %v", err)
	}

	firstQuery := first.Query()
	if firstQuery.Get("ft") != "20260605000000" || firstQuery.Get("to") != "20260605010000" || firstQuery.Get("seek") != "20260605000000" {
		t.Fatalf("unexpected first query: %s", first.RawQuery)
	}
	if firstQuery.Get("start_at") != "20260605000000" || firstQuery.Get("end_at") != "20260605010000" {
		t.Fatalf("unexpected first query: %s", first.RawQuery)
	}
	if got := last.Query().Get("seek"); got != "20260605005500" {
		t.Fatalf("last seek = %q, want %q", got, "20260605005500")
	}
}
