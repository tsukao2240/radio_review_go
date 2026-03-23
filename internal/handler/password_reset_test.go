package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubPasswordResetService は PasswordResetServiceInterface のスタブ実装。
type stubPasswordResetService struct {
	sendResetLinkFunc func(email string) error
	resetFunc         func(email, token, newPassword, newPasswordConfirmation string) error
}

func (s *stubPasswordResetService) SendResetLink(email string) error {
	if s.sendResetLinkFunc != nil {
		return s.sendResetLinkFunc(email)
	}
	return nil
}
func (s *stubPasswordResetService) Reset(email, token, newPassword, newPasswordConfirmation string) error {
	if s.resetFunc != nil {
		return s.resetFunc(email, token, newPassword, newPasswordConfirmation)
	}
	return nil
}

var _ service.PasswordResetServiceInterface = (*stubPasswordResetService)(nil)

func TestPasswordResetHandler_ShowRequestForm(t *testing.T) {
	h := NewPasswordResetHandler(&stubPasswordResetService{})
	req := httptest.NewRequest(http.MethodGet, "/password/reset", nil)
	rr := httptest.NewRecorder()
	h.ShowRequestForm(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestPasswordResetHandler_SendResetLink(t *testing.T) {
	t.Run("email 未指定: 200 (エラーメッセージ含む)", func(t *testing.T) {
		h := NewPasswordResetHandler(&stubPasswordResetService{})
		form := url.Values{"email": {""}}
		req := httptest.NewRequest(http.MethodPost, "/password/email", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.SendResetLink(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("メール送信成功: 200", func(t *testing.T) {
		var calledWith string
		svc := &stubPasswordResetService{
			sendResetLinkFunc: func(email string) error {
				calledWith = email
				return nil
			},
		}
		h := NewPasswordResetHandler(svc)
		form := url.Values{"email": {"user@example.com"}}
		req := httptest.NewRequest(http.MethodPost, "/password/email", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.SendResetLink(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if calledWith != "user@example.com" {
			t.Errorf("got %q, want user@example.com", calledWith)
		}
	})

	t.Run("サービスがエラーを返しても 200 を返す（列挙攻撃対策）", func(t *testing.T) {
		svc := &stubPasswordResetService{
			sendResetLinkFunc: func(email string) error {
				return errors.New("some error")
			},
		}
		h := NewPasswordResetHandler(svc)
		form := url.Values{"email": {"user@example.com"}}
		req := httptest.NewRequest(http.MethodPost, "/password/email", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.SendResetLink(rr, req)
		// サービスエラーは無視して成功メッセージを表示
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestPasswordResetHandler_ShowResetForm(t *testing.T) {
	h := NewPasswordResetHandler(&stubPasswordResetService{})
	r := chi.NewRouter()
	r.Get("/password/reset/{token}", h.ShowResetForm)
	req := httptest.NewRequest(http.MethodGet, "/password/reset/abc123?email=user@example.com", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestPasswordResetHandler_Reset(t *testing.T) {
	t.Run("正常リセット: /login?reset=1 にリダイレクト", func(t *testing.T) {
		h := NewPasswordResetHandler(&stubPasswordResetService{})
		form := url.Values{
			"email":                 {"user@example.com"},
			"token":                 {"abc123"},
			"password":              {"newpass123"},
			"password_confirmation": {"newpass123"},
		}
		req := httptest.NewRequest(http.MethodPost, "/password/update", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Reset(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login?reset=1" {
			t.Errorf("got Location=%q, want /login?reset=1", loc)
		}
	})

	t.Run("リセット失敗: 200 (エラーメッセージ含む)", func(t *testing.T) {
		svc := &stubPasswordResetService{
			resetFunc: func(_, _, _, _ string) error {
				return errors.New("invalid token")
			},
		}
		h := NewPasswordResetHandler(svc)
		form := url.Values{
			"email":                 {"user@example.com"},
			"token":                 {"wrongtoken"},
			"password":              {"newpass123"},
			"password_confirmation": {"newpass123"},
		}
		req := httptest.NewRequest(http.MethodPost, "/password/update", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Reset(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}
