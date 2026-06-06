package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler は認証関連のHTTPハンドラーを管理する。
type AuthHandler struct {
	userRepo repository.UserRepositoryInterface
	store    sessions.Store
}

// NewAuthHandler は新しい AuthHandler を返す。
func NewAuthHandler(userRepo repository.UserRepositoryInterface, store sessions.Store) *AuthHandler {
	return &AuthHandler{
		userRepo: userRepo,
		store:    store,
	}
}

// ShowLogin は GET /login を処理する。
// ログインフォームの HTML テンプレートを返す。
func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	if r.URL.Query().Get("reset") == "1" {
		data = map[string]interface{}{"ResetSuccess": true}
	}
	RenderWithBase(w, r, "web/templates/auth/login.html", data)
}

// Login は POST /login を処理する。
// email/password を検証し、成功したらセッションを保存して "/" にリダイレクトする。
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		// JSON リクエストも考慮
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr == nil {
			email = req.Email
			password = req.Password
		}
	}

	if email == "" || password == "" {
		writeError(w, http.StatusUnprocessableEntity, "email と password は必須です")
		return
	}

	user, err := h.userRepo.FindByEmail(email)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "メールアドレスまたはパスワードが正しくありません")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		writeError(w, http.StatusUnauthorized, "メールアドレスまたはパスワードが正しくありません")
		return
	}

	if err := middleware.SetUserInSession(r, w, h.store, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout は POST /logout を処理する。
// セッションをクリアして "/login" にリダイレクトする。
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := middleware.ClearSession(r, w, h.store); err != nil {
		writeError(w, http.StatusInternalServerError, "セッションのクリアに失敗しました")
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ShowRegister は GET /register を処理する。
// ユーザー登録フォームの HTML テンプレートを返す。
func (h *AuthHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	RenderWithBase(w, r, "web/templates/auth/register.html", nil)
}

// Register は POST /register を処理する。
// name/email/password/password_confirmation を検証し、ユーザーを作成してセッション保存後 "/" にリダイレクトする。
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")
	passwordConfirmation := r.FormValue("password_confirmation")

	if name == "" || email == "" || password == "" || passwordConfirmation == "" {
		// JSON リクエストも考慮
		var req struct {
			Name                 string `json:"name"`
			Email                string `json:"email"`
			Password             string `json:"password"`
			PasswordConfirmation string `json:"password_confirmation"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr == nil {
			if name == "" {
				name = req.Name
			}
			if email == "" {
				email = req.Email
			}
			if password == "" {
				password = req.Password
			}
			if passwordConfirmation == "" {
				passwordConfirmation = req.PasswordConfirmation
			}
		}
	}

	// バリデーション: 必須チェック
	if name == "" || email == "" || password == "" || passwordConfirmation == "" {
		SetFlash(r, w, "name, email, password, password_confirmation は必須です")
		http.Redirect(w, r, "/register", http.StatusFound)
		return
	}

	// バリデーション: パスワード一致確認
	if password != passwordConfirmation {
		SetFlash(r, w, "パスワードと確認用パスワードが一致しません")
		http.Redirect(w, r, "/register", http.StatusFound)
		return
	}

	// パスワードハッシュ化（cost: 12）
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "パスワードのハッシュ化に失敗しました")
		return
	}

	user := &model.User{
		Name:     name,
		Email:    email,
		Password: string(hashedPassword),
	}

	userID, err := h.userRepo.Create(user)
	if err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(err.Error(), "Duplicate entry") {
			SetFlash(r, w, "このメールアドレスはすでに登録されています")
			http.Redirect(w, r, "/register", http.StatusFound)
			return
		}
		writeError(w, http.StatusInternalServerError, "ユーザーの作成に失敗しました")
		return
	}

	if err := middleware.SetUserInSession(r, w, h.store, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}
