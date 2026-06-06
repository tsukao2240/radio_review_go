package main

import (
	"context"
	"fmt"
	"log"
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
	"github.com/redis/go-redis/v9"

	"github.com/yourname/radio_review_go/internal/handler"
	"github.com/yourname/radio_review_go/internal/job"
	appmiddleware "github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
	"github.com/yourname/radio_review_go/internal/service"
	"github.com/yourname/radio_review_go/pkg/radiko"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	// --- DB ---
	db := mustConnectDB()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// --- Redis ---
	rdb := mustConnectRedis()

	// --- Session store ---
	appKey := os.Getenv("APP_KEY")
	if appKey == "" {
		appKey = "change-me-in-production-32bytes!!"
	}
	store := sessions.NewCookieStore([]byte(appKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	// --- Repositories ---
	userRepo := repository.NewUserRepository(db)
	postRepo := repository.NewPostRepository(db)
	programRepo := repository.NewRadioProgramRepository(db)
	favRepo := repository.NewFavoriteProgramRepository(db)
	scheduleRepo := repository.NewRecordingScheduleRepository(db)
	tagRepo := repository.NewPostTagRepository(db)
	likeRepo := repository.NewPostLikeRepository(db)
	commentRepo := repository.NewPostCommentRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)

	// --- Services ---
	radikoSvc := service.NewRadikoApiService(rdb, programRepo)
	searchSvc := service.NewRadioProgramSearchService(programRepo, rdb)
	postSvc := service.NewPostService(postRepo, programRepo, tagRepo)
	interactionSvc := service.NewPostInteractionService(likeRepo, commentRepo)
	favSvc := service.NewFavoriteService(favRepo)
	notifSvc := service.NewNotificationService(notifRepo)
	scheduleSvc := service.NewRecordingScheduleService(scheduleRepo)
	recommendSvc := service.NewRecommendationService(postRepo, programRepo, favRepo, rdb)
	passwordResetSvc := service.NewPasswordResetService(passwordResetRepo, userRepo)

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
	authHandler := handler.NewAuthHandler(userRepo, store)
	broadcastHandler := handler.NewBroadcastHandler(radikoSvc, searchSvc, programRepo)
	postHandler := handler.NewPostHandler(postSvc, interactionSvc, store)
	mypageHandler := handler.NewMypageHandler(postSvc, store)
	favHandler := handler.NewFavoriteHandler(favSvc, radikoSvc, store)
	recordingHandler := handler.NewRecordingHandler(radikoClient, hlsDownloader, rdb, storagePath)
	notifHandler := handler.NewNotificationHandler(notifSvc, store)
	scheduleHandler := handler.NewScheduleHandler(scheduleSvc, store)
	recommendHandler := handler.NewRecommendationHandler(recommendSvc, store)
	passwordResetHandler := handler.NewPasswordResetHandler(passwordResetSvc)

	// --- フラッシュメッセージストアの登録 ---
	handler.FlashStore = store

	// --- ユーザー解決関数の登録 (RenderWithBase でナビに使用) ---
	handler.ResolveUser = func(r *http.Request) *model.User {
		session, err := store.Get(r, "radio_review_session")
		if err != nil {
			return nil
		}
		idVal, ok := session.Values["user_id"]
		if !ok {
			return nil
		}
		userID, ok := idVal.(int64)
		if !ok {
			return nil
		}
		user, err := userRepo.FindByID(userID)
		if err != nil {
			return nil
		}
		return user
	}

	// --- Background jobs ---
	scheduler := job.NewScheduler(db, rdb, radikoClient, hlsDownloader, storagePath)
	go scheduler.Start(ctx)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(appmiddleware.SecurityHeaders)

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
		http.NotFound(w, r) // ファビコン未作成につき 404
	})

	// 認証（ログインは10回/分のレートリミット）
	r.Get("/login", authHandler.ShowLogin)
	r.With(appmiddleware.RateLimit(10, time.Minute)).Post("/login", authHandler.Login)
	r.Post("/logout", authHandler.Logout)
	r.Get("/register", authHandler.ShowRegister)
	r.With(appmiddleware.RateLimit(10, time.Minute)).Post("/register", authHandler.Register)

	// パスワードリセット（レートリミット: 5回/分）
	r.Get("/password/reset", passwordResetHandler.ShowRequestForm)
	r.With(appmiddleware.RateLimit(5, time.Minute)).Post("/password/email", passwordResetHandler.SendResetLink)
	r.Get("/password/reset/{token}", passwordResetHandler.ShowResetForm)
	r.With(appmiddleware.RateLimit(5, time.Minute)).Post("/password/update", passwordResetHandler.Reset)

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

	// レビュー（公開）
	r.Get("/program", postHandler.IndexPrograms)
	r.Get("/review/list", postHandler.ListAllReviews)
	r.Get("/list/{station_id}/{title}/review", postHandler.ListReviewsByProgram)
	r.Get("/program/{program_id}/rating", postHandler.GetProgramRating)

	// レビュー（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/review/{id}", postHandler.ShowReviewForm)
		r.Post("/review/{id}", postHandler.CreateReview)
	})

	// マイページ（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/my", mypageHandler.Index)
		r.Get("/my/edit/{program_id}", mypageHandler.Edit)
		r.Post("/my/edit/{program_id}", mypageHandler.Update)
		r.Post("/my", mypageHandler.Destroy)
	})

	// お気に入り（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/favorites", favHandler.Index)
		r.Post("/favorites", favHandler.Store)
		r.Post("/favorites/delete", favHandler.Destroy)
		r.Get("/favorites/check", favHandler.Check)
	})

	// 録音（認証不要）
	r.Post("/recording/timefree/start", recordingHandler.StartTimefreeRecording(store))
	r.Post("/recording/stop", recordingHandler.StopRecording(store))
	r.Get("/recording/status", recordingHandler.GetRecordingStatus(store))
	r.Get("/recording/stream", recordingHandler.StreamRecording(store))
	r.Get("/recording/download", recordingHandler.DownloadRecording(store))
	r.Get("/recording/list", recordingHandler.ListRecordings(store))
	r.Get("/recording/history", recordingHandler.ShowHistory(store))
	r.Post("/recording/delete", recordingHandler.DeleteRecording(store))

	// 録音予約（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/recording/schedules", scheduleHandler.Index)
		r.Post("/recording/schedule", scheduleHandler.Store)
		r.Post("/recording/schedule/cancel", scheduleHandler.Cancel)
	})

	// 通知（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/notifications", notifHandler.Index)
		r.Get("/api/notifications/unread", notifHandler.GetUnread)
		r.Get("/api/notifications/all", notifHandler.GetAll)
		r.Post("/api/notifications/mark-read", notifHandler.MarkAsRead)
		r.Post("/api/notifications/mark-all-read", notifHandler.MarkAllAsRead)
	})

	// レコメンデーション（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Get("/recommendations", recommendHandler.Index)
		r.Get("/api/recommendations", recommendHandler.GetRecommendations)
		r.Post("/api/recommendations/refresh", recommendHandler.Refresh)
	})

	// 投稿インタラクション API（認証必須）
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(store))
		r.Post("/api/posts/like", postHandler.LikePost)
		r.Post("/api/posts/unlike", postHandler.UnlikePost)
		r.Post("/api/posts/comment", postHandler.AddComment)
		r.Post("/api/posts/comment/delete", postHandler.DeleteComment)
		r.Get("/api/posts/comments", postHandler.GetComments)
		r.Get("/api/posts/check-like", postHandler.CheckLike)
	})

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
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
		log.Fatalf("failed to open DB: %v", err)
	}
	return db
}

func mustConnectRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s",
			getEnvOrDefault("REDIS_HOST", "127.0.0.1"),
			getEnvOrDefault("REDIS_PORT", "6379"),
		),
	})
	return rdb
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
