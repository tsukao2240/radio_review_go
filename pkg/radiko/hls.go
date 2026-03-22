package radiko

import "context"

// HLSDownloaderInterface はHLSセグメントのダウンロード・結合インターフェース。
type HLSDownloaderInterface interface {
	// DownloadTimefree はタイムフリー番組をダウンロードしてファイルパスを返す。
	// errgroup + semaphore で最大RECORDING_MAX_PARALLEL並列（デフォルト10）。
	DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime string, outputPath string) error
}

// HLSDownloader はHLSダウンロードの実装。
type HLSDownloader struct {
	client     *Client
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

// DownloadTimefree は未実装（フェーズ2で実装）。
func (d *HLSDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime string, outputPath string) error {
	panic("not implemented: will be implemented in Phase 2")
}
