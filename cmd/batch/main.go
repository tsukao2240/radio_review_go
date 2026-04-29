package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/radio_review_go/internal/job"
	"github.com/yourname/radio_review_go/pkg/radiko"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
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

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "127.0.0.1"), getEnv("REDIS_PORT", "6379")),
	})

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
