package radiko

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// HLSDownloaderInterface はHLSセグメントのダウンロード・結合インターフェース。
type HLSDownloaderInterface interface {
	// DownloadTimefree はタイムフリー番組をダウンロードしてファイルパスを返す。
	// errgroup + semaphore で最大RECORDING_MAX_PARALLEL並列（デフォルト10）。
	DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error
}

// HLSDownloader はHLSダウンロードの実装。
type HLSDownloader struct {
	client      *Client
	maxParallel int64
}

// NewHLSDownloader は新しいHLSDownloaderを返す。
func NewHLSDownloader(client *Client, maxParallel int64) *HLSDownloader {
	if maxParallel <= 0 {
		maxParallel = 10
	}
	return &HLSDownloader{
		client:      client,
		maxParallel: maxParallel,
	}
}

// DownloadTimefree はタイムフリー番組をHLSセグメント並列ダウンロードで取得し、outputPath に書き込む。
func (d *HLSDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error {
	// 1. タイムフリーm3u8プレイリストURLの組み立て
	playlistURLs, err := d.buildTimefreePlaylistURLs(ctx, stationID, startTime, endTime, authToken, areaID)
	if err != nil {
		return fmt.Errorf("buildTimefreePlaylistURLs: %w", err)
	}

	// 2. m3u8を取得してセグメントURLを抽出
	seen := make(map[string]struct{})
	var segmentURLs []string
	for _, playlistURL := range playlistURLs {
		segments, err := d.fetchSegmentURLs(ctx, playlistURL, authToken)
		if err != nil {
			return fmt.Errorf("fetchSegmentURLs %s: %w", playlistURL, err)
		}
		for _, segmentURL := range segments {
			if _, ok := seen[segmentURL]; ok {
				continue
			}
			seen[segmentURL] = struct{}{}
			segmentURLs = append(segmentURLs, segmentURL)
		}
	}
	if len(segmentURLs) == 0 {
		return fmt.Errorf("no segments found in timefree playlists")
	}

	// 3. セグメントを並列ダウンロード
	results := make([][]byte, len(segmentURLs))
	g, gctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(d.maxParallel)

	for i, u := range segmentURLs {
		i, u := i, u
		g.Go(func() error {
			if err := sem.Acquire(gctx, 1); err != nil {
				return fmt.Errorf("sem.Acquire segment[%d]: %w", i, err)
			}
			defer sem.Release(1)

			data, err := d.fetchSegment(gctx, u, authToken)
			if err != nil {
				return fmt.Errorf("fetchSegment[%d] %s: %w", i, u, err)
			}
			results[i] = data
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel download: %w", err)
	}

	// 4. セグメントを順番に結合して outputPath に書き込む
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("os.Create %s: %w", outputPath, err)
	}
	defer func() { _ = f.Close() }()

	for i, data := range results {
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write segment[%d]: %w", i, err)
		}
	}

	return nil
}

// fetchSegmentURLs はm3u8プレイリストを取得し、セグメントURLの一覧を返す。
// 相対URLはplaylistURLをベースに絶対URLへ変換する。
func (d *HLSDownloader) fetchSegmentURLs(ctx context.Context, playlistURL, authToken string) ([]string, error) {
	return d.fetchSegmentURLsDepth(ctx, playlistURL, authToken, 0)
}

func (d *HLSDownloader) fetchSegmentURLsDepth(ctx context.Context, playlistURL, authToken string, depth int) ([]string, error) {
	if depth > 4 {
		return nil, fmt.Errorf("playlist nesting too deep: %s", playlistURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.Header.Set("X-Radiko-AuthToken", authToken)

	resp, err := d.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET playlist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET playlist returned status %d", resp.StatusCode)
	}

	base, err := url.Parse(playlistURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse playlistURL: %w", err)
	}

	var segments []string
	scanner := bufio.NewScanner(resp.Body)
	lastDirective := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			lastDirective = line
			continue
		}
		resolvedURL, err := resolveURL(base, line)
		if err != nil {
			return nil, fmt.Errorf("resolveURL %q: %w", line, err)
		}

		if isNestedPlaylistLine(lastDirective, resolvedURL) {
			nestedSegments, err := d.fetchSegmentURLsDepth(ctx, resolvedURL, authToken, depth+1)
			if err != nil {
				return nil, err
			}
			segments = append(segments, nestedSegments...)
		} else {
			segments = append(segments, resolvedURL)
		}
		lastDirective = ""
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner: %w", err)
	}

	return segments, nil
}

func isNestedPlaylistLine(lastDirective, resolvedURL string) bool {
	if strings.HasPrefix(lastDirective, "#EXT-X-STREAM-INF") {
		return true
	}
	lowerURL := strings.ToLower(resolvedURL)
	return strings.Contains(lowerURL, ".m3u8") || strings.Contains(lowerURL, "medialist")
}

func (d *HLSDownloader) buildTimefreePlaylistURLs(ctx context.Context, stationID, startTime, endTime, authToken, areaID string) ([]string, error) {
	startAt, err := normalizeRadikoTimestamp(startTime)
	if err != nil {
		return nil, fmt.Errorf("startTime: %w", err)
	}
	endAt, err := normalizeRadikoTimestamp(endTime)
	if err != nil {
		return nil, fmt.Errorf("endTime: %w", err)
	}

	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return nil, fmt.Errorf("time.LoadLocation: %w", err)
	}
	start, err := time.ParseInLocation("20060102150405", startAt, loc)
	if err != nil {
		return nil, fmt.Errorf("parse startTime: %w", err)
	}
	end, err := time.ParseInLocation("20060102150405", endAt, loc)
	if err != nil {
		return nil, fmt.Errorf("parse endTime: %w", err)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("endTime must be after startTime")
	}

	isAreaFree := strings.TrimSpace(areaID) != "" && areaID != "JP13"
	baseURL, err := d.fetchTimefreePlaylistCreateURL(ctx, stationID, authToken, isAreaFree)
	if err != nil {
		return nil, err
	}

	const chunkSeconds = 300
	lsid := randomLSID()
	playlistURLs := make([]string, 0, int(end.Sub(start)/(chunkSeconds*time.Second))+1)
	for seek := start; seek.Before(end); seek = seek.Add(chunkSeconds * time.Second) {
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("url.Parse playlist_create_url: %w", err)
		}
		q := u.Query()
		q.Set("station_id", stationID)
		q.Set("start_at", startAt)
		q.Set("ft", startAt)
		q.Set("end_at", endAt)
		q.Set("to", endAt)
		q.Set("seek", seek.Format("20060102150405"))
		q.Set("l", fmt.Sprintf("%d", chunkSeconds))
		q.Set("lsid", lsid)
		if isAreaFree {
			q.Set("type", "c")
		} else {
			q.Set("type", "b")
		}
		u.RawQuery = q.Encode()
		playlistURLs = append(playlistURLs, u.String())
	}

	return playlistURLs, nil
}

func normalizeRadikoTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch len(value) {
	case 12:
		value += "00"
	case 14:
	default:
		return "", fmt.Errorf("invalid timestamp length %d", len(value))
	}

	datePart := value[:8]
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return "", fmt.Errorf("time.LoadLocation: %w", err)
	}
	baseDate, err := time.ParseInLocation("20060102", datePart, loc)
	if err != nil {
		return "", fmt.Errorf("invalid date: %w", err)
	}

	hour, err := strconv.Atoi(value[8:10])
	if err != nil {
		return "", fmt.Errorf("invalid hour: %w", err)
	}
	if hour < 0 {
		return "", fmt.Errorf("invalid hour %d", hour)
	}
	minute, err := strconv.Atoi(value[10:12])
	if err != nil {
		return "", fmt.Errorf("invalid minute: %w", err)
	}
	second, err := strconv.Atoi(value[12:14])
	if err != nil {
		return "", fmt.Errorf("invalid second: %w", err)
	}
	if minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid minute %d", minute)
	}
	if second < 0 || second > 59 {
		return "", fmt.Errorf("invalid second %d", second)
	}

	normalized := baseDate.AddDate(0, 0, hour/24).Add(time.Duration(hour%24)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second)
	return normalized.Format("20060102150405"), nil
}

func randomLSID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func (d *HLSDownloader) fetchTimefreePlaylistCreateURL(ctx context.Context, stationID, authToken string, isAreaFree bool) (string, error) {
	streamInfoURL := fmt.Sprintf("https://radiko.jp/v3/station/stream/pc_html5/%s.xml", url.PathEscape(stationID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamInfoURL, nil)
	if err != nil {
		return "", fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	if authToken != "" {
		req.Header.Set("X-Radiko-AuthToken", authToken)
	}

	resp, err := d.client.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET stream info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET stream info returned status %d", resp.StatusCode)
	}

	playlistCreateURL, err := parseTimefreePlaylistCreateURL(resp.Body, isAreaFree)
	if err != nil {
		return "", err
	}
	return playlistCreateURL, nil
}

func parseTimefreePlaylistCreateURL(r io.Reader, isAreaFree bool) (string, error) {
	decoder := xml.NewDecoder(r)

	type candidate struct {
		text     string
		areafree bool
	}
	var candidates []candidate
	inURL := false
	inPlaylistCreateURL := false
	urlTimefree := false
	urlAreafree := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("xml decode stream info: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "url":
				inURL = true
				urlTimefree = false
				urlAreafree = false
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "timefree":
						urlTimefree = attr.Value == "1"
					case "areafree":
						urlAreafree = attr.Value == "1"
					}
				}
			case "playlist_create_url":
				if inURL && urlTimefree {
					inPlaylistCreateURL = true
				}
			}
		case xml.CharData:
			if inPlaylistCreateURL {
				text := strings.TrimSpace(string(t))
				if text != "" {
					candidates = append(candidates, candidate{text: text, areafree: urlAreafree})
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "playlist_create_url":
				inPlaylistCreateURL = false
			case "url":
				inURL = false
				urlTimefree = false
				urlAreafree = false
			}
		}
	}

	for _, candidate := range candidates {
		if candidate.areafree == isAreaFree {
			return candidate.text, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0].text, nil
	}
	return "", fmt.Errorf("timefree playlist_create_url not found")
}

// fetchSegment は1つのセグメントURLからバイト列を取得して返す。
func (d *HLSDownloader) fetchSegment(ctx context.Context, segURL, authToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, segURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.Header.Set("X-Radiko-AuthToken", authToken)

	resp, err := d.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET segment: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET segment returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll: %w", err)
	}
	return data, nil
}

// resolveURL はベースURLに対してrawURLを解決し、絶対URLの文字列を返す。
func resolveURL(base *url.URL, rawURL string) (string, error) {
	ref, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}
