package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

func TestResolveAppKey(t *testing.T) {
	t.Run("production requires APP_KEY", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("APP_KEY", "")
		if _, err := resolveAppKey(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("local falls back", func(t *testing.T) {
		t.Setenv("APP_ENV", "local")
		t.Setenv("APP_KEY", "")
		got, err := resolveAppKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == "" {
			t.Fatal("expected fallback key")
		}
	})
}

func TestWriteRouteRateLimitsConfigured(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile main.go: %v", err)
	}
	body := string(src)
	required := []string{
		`RateLimit(10, time.Minute)).Post("/review/{id}"`,
		`RateLimit(20, time.Minute)).Post("/api/posts/comment"`,
		`RateLimit(30, time.Minute)).Post("/favorites"`,
		`RateLimit(30, time.Minute)).Post("/favorites/delete"`,
		`RateLimit(2, time.Minute)).Post("/favorites/record-all"`,
		`RateLimit(5, time.Minute)).Post("/recording/timefree/start"`,
	}
	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Fatalf("missing rate limit route config: %s", needle)
		}
	}
}

func TestNewHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux())
	if srv.ReadTimeout != 10*time.Second {
		t.Fatalf("ReadTimeout got %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout got %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout got %v", srv.IdleTimeout)
	}
}

func TestHealthzHandler(t *testing.T) {
	rawDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "sqlmock")
	mock.ExpectPing()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	healthzHandler(db, rdb).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	rawDB2, mock2, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New second: %v", err)
	}
	defer rawDB2.Close()
	db2 := sqlx.NewDb(rawDB2, "sqlmock")
	mock2.ExpectPing().WillReturnError(context.DeadlineExceeded)

	rr = httptest.NewRecorder()
	healthzHandler(db2, rdb).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rr.Code)
	}
}
