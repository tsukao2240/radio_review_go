package radiko

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// HLSDownloaderInterface はHLSセグメントのダウンロード・結合インターフェース。
type HLSDownloaderInterface interface {
	// DownloadTimefree はタイムフリー番組をダウンロードしてファイルパスを返す。
	// errgroup + semaphore で最大RECORDING_MAX_PARALLEL並列（デフォルト10）。
	DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime string, outputPath string) error
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
func (d *HLSDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime string, outputPath string) error {
	// 1. タイムフリーm3u8プレイリストURLの組み立て
	playlistURL := fmt.Sprintf(
		"https://radiko.jp/v2/api/ts/playlist.m3u8?station_id=%s&ft=%s&to=%s&l=15",
		stationID, startTime, endTime,
	)

	// 2. m3u8を取得してセグメントURLを抽出
	segmentURLs, err := d.fetchSegmentURLs(ctx, playlistURL, authToken)
	if err != nil {
		return fmt.Errorf("fetchSegmentURLs: %w", err)
	}
	if len(segmentURLs) == 0 {
		return fmt.Errorf("no segments found in playlist: %s", playlistURL)
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
	defer f.Close()

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.Header.Set("X-Radiko-AuthToken", authToken)

	resp, err := d.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET playlist returned status %d", resp.StatusCode)
	}

	base, err := url.Parse(playlistURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse playlistURL: %w", err)
	}

	var segments []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// コメント行・空行・ディレクティブ行はスキップ
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// .ts セグメントURL（絶対・相対どちらも対応）
		segURL, err := resolveURL(base, line)
		if err != nil {
			return nil, fmt.Errorf("resolveURL %q: %w", line, err)
		}
		segments = append(segments, segURL)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner: %w", err)
	}

	return segments, nil
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
	defer resp.Body.Close()

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
