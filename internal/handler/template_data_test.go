package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/sessions"
)

func TestRenderWithBase_Success(t *testing.T) {
	// Create a temporary directory structure with fake templates
	dir := t.TempDir()

	// Create web/templates/layouts/base.html
	baseDir := filepath.Join(dir, "web/templates/layouts")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	baseContent := `{{define "base"}}BASE:{{block "content" .}}{{end}}{{end}}`
	if err := os.WriteFile(filepath.Join(baseDir, "base.html"), []byte(baseContent), 0600); err != nil {
		t.Fatalf("WriteFile base: %v", err)
	}

	// Create the page template
	pageContent := `{{define "content"}}PAGE{{end}}`
	pagePath := filepath.Join(dir, "page.html")
	if err := os.WriteFile(pagePath, []byte(pageContent), 0600); err != nil {
		t.Fatalf("WriteFile page: %v", err)
	}

	// Change to temp dir so baseTmpl ("web/templates/layouts/base.html") resolves
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	// Restore working directory after test
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	RenderWithBase(rr, req, pagePath, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", ct)
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	token := generateCSRFToken()
	if token == "" {
		t.Error("expected non-empty CSRF token")
	}
	// Should be valid base64
	if len(token) < 20 {
		t.Errorf("expected token length >= 20, got %d", len(token))
	}
	// Two calls should produce different tokens
	token2 := generateCSRFToken()
	if token == token2 {
		t.Error("expected different tokens on each call")
	}
}

func TestGetAndClearFlash(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	FlashStore = store
	defer func() { FlashStore = nil }()

	t.Run("フラッシュなしの場合: 空文字を返す", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		msg := getAndClearFlash(req, rr)
		if msg != "" {
			t.Errorf("expected empty string, got %q", msg)
		}
	})

	t.Run("フラッシュあり: メッセージを返してクリアする", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		// Set flash first
		SetFlash(req, rr, "テストメッセージ")

		// Create new request with the session cookie from the response
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, cookie := range rr.Result().Cookies() {
			req2.AddCookie(cookie)
		}
		rr2 := httptest.NewRecorder()

		msg := getAndClearFlash(req2, rr2)
		if msg != "テストメッセージ" {
			t.Errorf("expected 'テストメッセージ', got %q", msg)
		}

		// Second read should return empty (cleared)
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, cookie := range rr2.Result().Cookies() {
			req3.AddCookie(cookie)
		}
		rr3 := httptest.NewRecorder()
		msg2 := getAndClearFlash(req3, rr3)
		if msg2 != "" {
			t.Errorf("expected empty after clear, got %q", msg2)
		}
	})
}

func TestSetFlash_NilStore(t *testing.T) {
	FlashStore = nil
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	// Should not panic
	SetFlash(req, rr, "msg")
}

func TestGetAndClearFlash_NilStore(t *testing.T) {
	FlashStore = nil
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	msg := getAndClearFlash(req, rr)
	if msg != "" {
		t.Errorf("expected empty for nil store, got %q", msg)
	}
}

func TestRenderWithBase_ParseFilesError(t *testing.T) {
	// Create a temp file that exists but base template doesn't → ParseFiles fails
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "page.html")
	if err := os.WriteFile(tmplPath, []byte(`{{define "content"}}test{{end}}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	RenderWithBase(rr, req, tmplPath, nil)

	// ParseFiles fails because baseTmpl ("web/templates/layouts/base.html") doesn't exist
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestRenderWithBase_JSONFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	// Use a path that definitely doesn't exist
	RenderWithBase(rr, req, "nonexistent/template.html", data)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["key"] != "value" {
		t.Errorf("expected key=value, got %q", resp["key"])
	}
}
