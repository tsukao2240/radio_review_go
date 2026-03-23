package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
)

func TestGetUserID(t *testing.T) {
	t.Run("認証済みコンテキスト: userID を返す", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserContextKey, int64(42))
		id, ok := GetUserID(ctx)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if id != 42 {
			t.Errorf("got %d, want 42", id)
		}
	})

	t.Run("未認証コンテキスト: ok=false を返す", func(t *testing.T) {
		_, ok := GetUserID(context.Background())
		if ok {
			t.Fatal("expected ok=false for unauthenticated context")
		}
	})

	t.Run("型が int64 でない値: ok=false を返す", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserContextKey, "not-an-int64")
		_, ok := GetUserID(ctx)
		if ok {
			t.Fatal("expected ok=false for wrong type")
		}
	})
}

func TestRequireAuth_RedirectsWhenUnauthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	mw := RequireAuth(store)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if nextCalled {
		t.Error("next handler should not be called for unauthenticated request")
	}
	if rr.Code != http.StatusFound {
		t.Errorf("got %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("got Location=%q, want /login", loc)
	}
}

func TestRequireAuth_SetsUserIDInContext(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))

	// セッションにユーザーIDをセット
	setupReq := httptest.NewRequest(http.MethodGet, "/setup", nil)
	setupRR := httptest.NewRecorder()
	if err := SetUserInSession(setupReq, setupRR, store, int64(7)); err != nil {
		t.Fatalf("SetUserInSession: %v", err)
	}

	// セッションクッキーを取得
	cookie := setupRR.Result().Cookies()

	// 認証済みリクエストを作成
	mw := RequireAuth(store)
	var capturedUserID int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := GetUserID(r.Context())
		if !ok {
			t.Error("expected userID in context")
		}
		capturedUserID = id
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, c := range cookie {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	if capturedUserID != 7 {
		t.Errorf("got userID=%d, want 7", capturedUserID)
	}
}

func TestClearSession(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))

	// まずセッションをセット
	setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
	setupRR := httptest.NewRecorder()
	if err := SetUserInSession(setupReq, setupRR, store, int64(5)); err != nil {
		t.Fatalf("SetUserInSession: %v", err)
	}
	cookie := setupRR.Result().Cookies()

	// クリア
	clearReq := httptest.NewRequest(http.MethodGet, "/logout", nil)
	for _, c := range cookie {
		clearReq.AddCookie(c)
	}
	clearRR := httptest.NewRecorder()
	if err := ClearSession(clearReq, clearRR, store); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}

	// クリア後のクッキーで認証ミドルウェアを通すと /login にリダイレクトされる
	clearedCookies := clearRR.Result().Cookies()
	mw := RequireAuth(store)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, c := range clearedCookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("after ClearSession got %d, want 302 (redirect to /login)", rr.Code)
	}
}
