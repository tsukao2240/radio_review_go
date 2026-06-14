package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/tsukao2240/radio_review_go/internal/job"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/recordingfile"
	"github.com/tsukao2240/radio_review_go/pkg/radiko"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
	}
	if len(os.Args) > 1 && os.Args[1] == "rename-recordings" {
		if err := runRenameRecordings(context.Background(), os.Args[2:]); err != nil {
			log.Fatalf("録音ファイルリネーム失敗: %v", err)
		}
		return
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		getEnv("DB_HOST", "127.0.0.1"),
		getEnv("DB_PORT", "3306"),
		getEnv("DB_DATABASE", "radio_review"),
	)
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("DB接続失敗: %v", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)

	rdb := newRedisClient()

	keyPath := "storage/keys/radiko_auth_key.txt"
	radikoClient := radiko.NewClient(rdb, keyPath)
	hlsDownloader := radiko.NewHLSDownloader(radikoClient, 10)

	storagePath := getEnv("RECORDING_STORAGE_PATH", "storage/recordings")
	scheduler := job.NewScheduler(db, rdb, radikoClient, hlsDownloader, storagePath)

	log.Println("番組データ取得・保存を開始します...")
	scheduler.InsertRadioPrograms(context.Background())
	log.Println("完了しました。")
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "127.0.0.1"), getEnv("REDIS_PORT", "6379")),
	})
}

func runRenameRecordings(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rename-recordings", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", true, "変更内容を表示するだけでリネームしない")
	execute := fs.Bool("execute", false, "実際にファイル名とRedisメタを更新する")
	storagePath := fs.String("storage-path", getEnv("RECORDING_STORAGE_PATH", "storage/recordings"), "録音ファイル保存ディレクトリ")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *execute {
		*dryRun = false
	}

	rdb := newRedisClient()
	defer func() { _ = rdb.Close() }()

	result, err := renameRecordings(ctx, rdb, *storagePath, *dryRun)
	if err != nil {
		return err
	}
	log.Printf("rename-recordings completed dry_run=%t scanned=%d renamed=%d skipped=%d failed=%d",
		*dryRun, result.scanned, result.renamed, result.skipped, result.failed)
	return nil
}

type renameRecordingsResult struct {
	scanned int
	renamed int
	skipped int
	failed  int
}

func renameRecordings(ctx context.Context, rdb *redis.Client, storagePath string, dryRun bool) (renameRecordingsResult, error) {
	var result renameRecordingsResult
	var cursor uint64
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "recording_*", 500).Result()
		if err != nil {
			return result, fmt.Errorf("redis Scan: %w", err)
		}
		for _, key := range keys {
			result.scanned++
			ok, err := renameRecording(ctx, rdb, storagePath, key, dryRun)
			switch {
			case err != nil:
				result.failed++
				log.Printf("rename-recordings: failed key=%s error=%v", key, err)
			case ok:
				result.renamed++
			default:
				result.skipped++
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return result, nil
}

func renameRecording(ctx context.Context, rdb *redis.Client, storagePath, key string, dryRun bool) (bool, error) {
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return false, err
	}
	var info model.RecordingInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return false, nil
	}
	if info.FilePath == "" || info.StartTime == "" || info.StationID == "" || info.ProgramName == "" {
		log.Printf("rename-recordings: skip key=%s reason=metadata_missing", key)
		return false, nil
	}
	if !isLegacyRecordingName(filepath.Base(info.FilePath)) {
		return false, nil
	}
	if !isPathUnderStorage(storagePath, info.FilePath) {
		log.Printf("rename-recordings: skip key=%s reason=path_outside_storage path=%s", key, info.FilePath)
		return false, nil
	}
	if _, err := os.Stat(info.FilePath); err != nil {
		log.Printf("rename-recordings: skip key=%s reason=file_missing path=%s", key, info.FilePath)
		return false, nil
	}
	newPath := strings.TrimSuffix(recordingfile.NewPath(storagePath, info.RecordingID, info.StartTime, info.StationID, info.ProgramName), ".m4a") + ".aac"
	if samePath(info.FilePath, newPath) {
		return false, nil
	}
	if dryRun {
		log.Printf("rename-recordings: dry-run key=%s from=%s to=%s", key, info.FilePath, newPath)
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return false, err
	}
	if err := os.Rename(info.FilePath, newPath); err != nil {
		return false, err
	}
	info.FilePath = newPath
	data, err := json.Marshal(&info)
	if err != nil {
		return false, err
	}
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if ttl < 0 {
		ttl = 0
	}
	if err := rdb.Set(ctx, key, string(data), ttl).Err(); err != nil {
		return false, err
	}
	return true, nil
}

func isLegacyRecordingName(name string) bool {
	if filepath.Ext(name) != ".aac" {
		return false
	}
	underscore := strings.IndexByte(name, '_')
	if underscore <= 0 {
		return false
	}
	for _, r := range name[:underscore] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isPathUnderStorage(storagePath, path string) bool {
	storageAbs, err := filepath.Abs(storagePath)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(storageAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return absA == absB
}
