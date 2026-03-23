package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
)

// stubUserRepo は UserRepositoryInterface のスタブ実装。
type stubUserRepo struct {
	findByIDFunc    func(id int64) (*model.User, error)
	findByEmailFunc func(email string) (*model.User, error)
	createFunc      func(user *model.User) (int64, error)
	updateFunc      func(user *model.User) error
}

func (r *stubUserRepo) FindByID(id int64) (*model.User, error) {
	if r.findByIDFunc != nil {
		return r.findByIDFunc(id)
	}
	return nil, nil
}
func (r *stubUserRepo) FindByEmail(email string) (*model.User, error) {
	if r.findByEmailFunc != nil {
		return r.findByEmailFunc(email)
	}
	return nil, nil
}
func (r *stubUserRepo) Create(user *model.User) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(user)
	}
	return 1, nil
}
func (r *stubUserRepo) Update(user *model.User) error {
	if r.updateFunc != nil {
		return r.updateFunc(user)
	}
	return nil
}

var _ repository.UserRepositoryInterface = (*stubUserRepo)(nil)

func TestAuthHandler_ShowLogin(t *testing.T) {
	t.Run("GETリクエスト: 200", func(t *testing.T) {
		h := NewAuthHandler(&stubUserRepo{}, sessions.NewCookieStore([]byte("test")))
		req := httptest.NewRequest(http.MethodGet, "/login", nil)
		rr := httptest.NewRecorder()
		h.ShowLogin(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestAuthHandler_ShowLogin_ResetSuccess(t *testing.T) {
	h := NewAuthHandler(&stubUserRepo{}, sessions.NewCookieStore([]byte("test")))
	req := httptest.NewRequest(http.MethodGet, "/login?reset=1", nil)
	rr := httptest.NewRecorder()
	h.ShowLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestAuthHandler_Login(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))

	t.Run("email/password 未入力: 422", func(t *testing.T) {
		h := NewAuthHandler(&stubUserRepo{}, store)
		form := url.Values{"email": {""}, "password": {""}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("メールアドレスが存在しない: 401", func(t *testing.T) {
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return nil, nil
			},
		}
		h := NewAuthHandler(userRepo, store)
		form := url.Values{"email": {"notfound@example.com"}, "password": {"password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("パスワード不一致: 401", func(t *testing.T) {
		// bcrypt ハッシュ化済みパスワード "correct_password"
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return &model.User{
					ID:       1,
					Email:    email,
					Password: "$2a$12$invalidhashjustfortesting000000000000000000000000000000",
				}, nil
			},
		}
		h := NewAuthHandler(userRepo, store)
		form := url.Values{"email": {"user@example.com"}, "password": {"wrong_password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})
}

func TestAuthHandler_Login_JSONBody(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	// Return nil user (not found) to test the JSON decode path
	userRepo := &stubUserRepo{
		findByEmailFunc: func(email string) (*model.User, error) {
			return nil, nil
		},
	}
	h := NewAuthHandler(userRepo, store)
	body, _ := json.Marshal(map[string]string{
		"email":    "json@example.com",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	// user not found → 401
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Register(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))

	t.Run("必須フィールド未入力: 422", func(t *testing.T) {
		h := NewAuthHandler(&stubUserRepo{}, store)
		form := url.Values{"name": {""}, "email": {""}, "password": {""}, "password_confirmation": {""}}
		req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("パスワード不一致: 422", func(t *testing.T) {
		h := NewAuthHandler(&stubUserRepo{}, store)
		form := url.Values{
			"name":                  {"Alice"},
			"email":                 {"alice@example.com"},
			"password":              {"pass1234"},
			"password_confirmation": {"different"},
		}
		req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("正常登録: /にリダイレクト", func(t *testing.T) {
		userRepo := &stubUserRepo{
			createFunc: func(user *model.User) (int64, error) { return 1, nil },
		}
		h := NewAuthHandler(userRepo, store)
		form := url.Values{
			"name":                  {"Alice"},
			"email":                 {"alice@example.com"},
			"password":              {"pass1234"},
			"password_confirmation": {"pass1234"},
		}
		req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/" {
			t.Errorf("got Location=%q, want /", loc)
		}
	})
}

func TestAuthHandler_Register_CreateError(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	userRepo := &stubUserRepo{
		createFunc: func(user *model.User) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	h := NewAuthHandler(userRepo, store)
	form := url.Values{
		"name":                  {"Alice"},
		"email":                 {"alice@example.com"},
		"password":              {"pass1234"},
		"password_confirmation": {"pass1234"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestAuthHandler_Register_JSONBody(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	userRepo := &stubUserRepo{
		createFunc: func(user *model.User) (int64, error) { return 2, nil },
	}
	h := NewAuthHandler(userRepo, store)
	body, _ := json.Marshal(map[string]string{
		"name":                  "Bob",
		"email":                 "bob@example.com",
		"password":              "secret123",
		"password_confirmation": "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	if rr.Code != http.StatusFound {
		t.Errorf("got %d, want 302", rr.Code)
	}
}

// stubFailStore は sessions.Get でエラーを返すスタブストア
type stubFailStore struct{}

func (s *stubFailStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return nil, errors.New("store get error")
}
func (s *stubFailStore) New(r *http.Request, name string) (*sessions.Session, error) {
	return nil, errors.New("store new error")
}
func (s *stubFailStore) Save(r *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	return errors.New("store save error")
}

func TestAuthHandler_Logout_StoreError(t *testing.T) {
	h := NewAuthHandler(&stubUserRepo{}, &stubFailStore{})
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rr := httptest.NewRecorder()
	h.Logout(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))

	t.Run("/loginにリダイレクト", func(t *testing.T) {
		h := NewAuthHandler(&stubUserRepo{}, store)
		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		rr := httptest.NewRecorder()
		h.Logout(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login" {
			t.Errorf("got Location=%q, want /login", loc)
		}
	})
}

func TestAuthHandler_ShowRegister(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	h := NewAuthHandler(&stubUserRepo{}, store)
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	rr := httptest.NewRecorder()
	h.ShowRegister(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	// Generate a real bcrypt hash for "password123"
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 4) // cost=4 for speed
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	userRepo := &stubUserRepo{
		findByEmailFunc: func(email string) (*model.User, error) {
			return &model.User{ID: 42, Email: email, Password: string(hash)}, nil
		},
	}
	h := NewAuthHandler(userRepo, store)
	form := url.Values{"email": {"user@example.com"}, "password": {"password123"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	if rr.Code != http.StatusFound {
		t.Errorf("got %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("got Location=%q, want /", loc)
	}
}
