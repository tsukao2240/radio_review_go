package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/tsukao2240/radio_review_go/internal/handler"
	"github.com/tsukao2240/radio_review_go/internal/job"
	appmetrics "github.com/tsukao2240/radio_review_go/internal/metrics"
	appmiddleware "github.com/tsukao2240/radio_review_go/internal/middleware"
	"github.com/tsukao2240/radio_review_go/internal/migration"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
	"github.com/tsukao2240/radio_review_go/internal/service"
	"github.com/tsukao2240/radio_review_go/pkg/radiko"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	envLoadErr := godotenv.Load()
	configureLogger()
	if envLoadErr != nil {
		slog.Info("no .env file found, using environment variables")
	}

	// --- DB ---
	db := mustConnectDB()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := migration.Run(ctx, db); err != nil {
		fatal("database migration failed", err)
	}

	// --- Redis ---
	rdb := mustConnectRedis()

	// --- Session store ---
	appKey, err := resolveAppKey()
	if err != nil {
		fatal("resolve APP_KEY failed", err)
	}
	store := sessions.NewCookieStore([]byte(appKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if os.Getenv("APP_ENV") == "production" {
		store.Options.Secure = true
	}

	// --- Repositories ---
	userRepo := repository.NewUserRepository(db)
	programRepo := repository.NewRadioProgramRepository(db)
	favRepo := repository.NewFavoriteProgramRepository(db)
	scheduleRepo := repository.NewRecordingScheduleRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	pushRepo := repository.NewPushSubscriptionRepository(db)

	// --- Services ---
	radikoSvc := service.NewRadikoApiService(rdb, programRepo)
	searchSvc := service.NewRadioProgramSearchService(programRepo, rdb)
	favSvc := service.NewFavoriteService(favRepo)
	notifSvc := service.NewNotificationService(notifRepo)
	pushSvc := service.NewPushServiceFromEnv(pushRepo)
	notifSvc.SetPushSender(pushSvc)
	scheduleSvc := service.NewRecordingScheduleService(scheduleRepo)

	// --- Radiko client ---
	keyPath := "storage/keys/radiko_auth_key.txt"
	radikoClient := radiko.NewClient(rdb, keyPath)

	maxParallel := int64(10)
	if v := os.Getenv("RECORDING_MAX_PARALLEL"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxParallel = n
		}
	}
	hlsDownloader := radiko.NewHLSDownloader(radikoClient, maxParallel)

	storagePath := os.Getenv("RECORDING_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "storage/recordings"
	}

	// --- Handlers ---
	broadcastHandler := handler.NewBroadcastHandler(radikoSvc, searchSvc, programRepo)
	favHandler := handler.NewFavoriteHandler(favSvc, radikoSvc, store)
	recordingHandler := handler.NewRecordingHandler(radikoClient, hlsDownloader, rdb, storagePath)
	recordingHandler.SetUserRepository(userRepo)
	recordingHandler.SetDownloadCompletion(func(ctx context.Context, info *model.RecordingInfo) error {
		return recordDownloadCompletion(ctx, db, info)
	})
	favHandler.SetRecorder(recordingHandler)
	notifHandler := handler.NewNotificationHandler(notifSvc, store)
	pushHandler := handler.NewPushHandler(pushSvc)
	scheduleHandler := handler.NewScheduleHandler(scheduleSvc, store)

	// --- フラッシュメッセージストアの登録 ---
	handler.FlashStore = store

	// このPCでは既存のお気に入りを持つユーザーをログイン不要で利用する。
	var localUserID int64
	if err := db.Get(&localUserID, `SELECT u.id FROM users u
 ORDER BY (SELECT COUNT(*) FROM favorite_programs f WHERE f.user_id = u.id) DESC, u.id ASC LIMIT 1`); err != nil {
		fatal("resolve local user failed", err)
	}
	handler.ResolveUser = func(r *http.Request) *model.User {
		user, err := userRepo.FindByID(localUserID)
		if err != nil {
			return nil
		}
		return user
	}

	// --- Background jobs ---
	scheduler := job.NewScheduler(db, rdb, radikoClient, hlsDownloader, storagePath)
	scheduler.SetNotificationService(notifSvc)
	scheduler.SetFavoriteScheduleService(radikoSvc)
	go scheduler.Start(ctx)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(appmetrics.HTTPMiddleware)
	r.Use(appmiddleware.SecurityHeaders)
	r.Use(appmiddleware.LocalUser(localUserID))
	r.Use(appmiddleware.CSRFProtection(store))

	// 静的ファイル
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	// Vite ビルド成果物のフォントパス互換 (CSSが /build/assets/ を参照)
	r.Handle("/build/assets/*", http.StripPrefix("/build/assets/", http.FileServer(http.Dir("web/static"))))
	// PWA マニフェスト・Service Worker・ファビコン
	r.Get("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/manifest.json")
	})
	r.Get("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeFile(w, r, "web/static/sw.js")
	})
	r.Get("/offline.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/offline.html")
	})
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/icons/icon-192x192.png")
	})
	r.Get("/healthz", healthzHandler(db, rdb))
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/api/live", handler.LiveUpdates(db, rdb))
	r.Get("/api/csrf", appmiddleware.CSRFToken(store))
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
		handler.RenderWithBase(w, r, "web/templates/settings/download.html", nil)
	})

	// 古いPWAのログイン画面URLからも、そのままアプリへ移動する。
	for _, path := range []string{"/login", "/register", "/password/reset"} {
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/favorites", http.StatusSeeOther)
		})
	}

	// トップ
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/schedule", http.StatusFound)
	})

	// 番組表・検索（検索は30回/分のレートリミット）
	r.Get("/schedule", broadcastHandler.GetCurrentSchedule)
	r.Get("/schedule/{station_id}", broadcastHandler.GetWeeklySchedule)
	r.Get("/timefree", broadcastHandler.GetTwoWeekScheduleSelect)
	r.Get("/timefree/{station_id}", broadcastHandler.GetTwoWeekScheduleByStation)
	r.Get("/list/{station_id}/{title}", broadcastHandler.ShowProgramDetail)
	r.With(appmiddleware.RateLimit(30, time.Minute)).Get("/search", broadcastHandler.Search)

	// お気に入り（ログイン不要）
	r.Group(func(r chi.Router) {
		r.Get("/favorites", favHandler.Index)
		r.With(appmiddleware.RateLimit(30, time.Minute)).Post("/favorites", favHandler.Store)
		r.With(appmiddleware.RateLimit(30, time.Minute)).Post("/favorites/delete", favHandler.Destroy)
		r.With(appmiddleware.RateLimit(2, time.Minute)).Post("/favorites/record-all", favHandler.RecordAll)
		r.Get("/favorites/check", favHandler.Check)
	})

	// 録音（認証不要）
	// owner_key はログイン済みなら user_id、ゲストならセッションIDから生成し、
	// ゲスト録音を維持しつつ録音一覧・履歴を同一所有者に限定する。
	r.With(appmiddleware.RateLimit(5, time.Minute)).Post("/recording/timefree/start", recordingHandler.StartTimefreeRecording(store))
	r.Post("/recording/stop", recordingHandler.StopRecording(store))
	r.Get("/recording/status", recordingHandler.GetRecordingStatus(store))
	r.Get("/recording/stream", recordingHandler.StreamRecording(store))
	r.Get("/recording/download", recordingHandler.DownloadRecording(store))
	r.Post("/recording/download/complete", recordingHandler.CompleteSavedDownload(store))
	r.Get("/recording/feed.xml", recordingHandler.FeedXML)
	r.Get("/recording/list", recordingHandler.ListRecordings(store))
	r.Get("/recording/history", recordingHandler.ShowHistory(store))
	r.Post("/recording/delete", recordingHandler.DeleteRecording(store))

	// 録音予約（ログイン不要）
	r.Group(func(r chi.Router) {
		r.Get("/recording/schedules", scheduleHandler.Index)
		r.Post("/recording/schedule", scheduleHandler.Store)
		r.Post("/recording/schedule/cancel", scheduleHandler.Cancel)
	})

	// 通知（ログイン不要）
	r.Group(func(r chi.Router) {
		r.Get("/notifications", notifHandler.Index)
		r.Get("/api/notifications/unread", notifHandler.GetUnread)
		r.Get("/api/notifications/all", notifHandler.GetAll)
		r.Post("/api/notifications/mark-read", notifHandler.MarkAsRead)
		r.Post("/api/notifications/mark-all-read", notifHandler.MarkAllAsRead)
		r.Get("/api/push/vapid-public-key", pushHandler.VAPIDPublicKey)
		r.Post("/api/push/subscribe", pushHandler.Subscribe)
		r.Delete("/api/push/subscribe", pushHandler.Unsubscribe)
	})

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	srv := newHTTPServer("127.0.0.1:"+port, r)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	slog.Info("server starting", "addr", ":"+port)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fatal("server failed", err)
		}
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}

	if err := rdb.Close(); err != nil {
		slog.Error("redis close error", "error", err)
	}
	if err := db.Close(); err != nil {
		slog.Error("db close error", "error", err)
	}
}

func configureLogger() {
	level := slog.LevelDebug
	if os.Getenv("APP_ENV") == "production" {
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func fatal(message string, err error) {
	slog.Error(message, "error", err)
	os.Exit(1)
}

func resolveAppKey() (string, error) {
	appKey := os.Getenv("APP_KEY")
	if appKey != "" {
		return appKey, nil
	}
	if os.Getenv("APP_ENV") == "production" {
		return "", fmt.Errorf("APP_KEY is required when APP_ENV=production")
	}
	slog.Warn("APP_KEY is not set; using insecure default key for non-production")
	return "change-me-in-production-32bytes!!", nil
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func healthzHandler(db *sqlx.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}

func mustConnectDB() *sqlx.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		getEnvOrDefault("DB_HOST", "127.0.0.1"),
		getEnvOrDefault("DB_PORT", "3306"),
		getEnvOrDefault("DB_DATABASE", "radio_review"),
	)
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		fatal("failed to open DB", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fatal("failed to ping DB", err)
	}
	return db
}

func mustConnectRedis() *redis.Client {
	db := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fatal("invalid REDIS_DB", err)
		}
		db = n
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s",
			getEnvOrDefault("REDIS_HOST", "127.0.0.1"),
			getEnvOrDefault("REDIS_PORT", "6379"),
		),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})
	return rdb
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func recordDownloadCompletion(ctx context.Context, db *sqlx.DB, info *model.RecordingInfo) error {
	// 完了行と自動録音の一意キーを残し、Redis消失後も再録音・復元させない。
	_, err := db.ExecContext(ctx, `UPDATE recording_schedules
   SET recording_metadata = JSON_SET(COALESCE(recording_metadata, JSON_OBJECT()),
    '$.status', 'downloaded', '$.file_path', '', '$.downloaded_at', ?), updated_at = NOW()
   WHERE recording_id = ? AND status = 'completed'`, time.Now().Format(time.RFC3339), info.RecordingID)
	return err
}
