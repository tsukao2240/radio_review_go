package handler

import (
	"encoding/json"
	"html/template"
	"net/http"

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
	tmpl, err := template.ParseFiles("web/templates/auth/login.html")
	if err != nil {
		// テンプレートが存在しない場合はシンプルな HTML を直書き
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><title>ログイン</title></head>
<body>
<h1>ログイン</h1>
<form method="POST" action="/login">
  <label>メールアドレス: <input type="email" name="email" required></label><br>
  <label>パスワード: <input type="password" name="password" required></label><br>
  <button type="submit">ログイン</button>
</form>
<p><a href="/register">新規登録はこちら</a></p>
</body>
</html>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, nil)
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
	tmpl, err := template.ParseFiles("web/templates/auth/register.html")
	if err != nil {
		// テンプレートが存在しない場合はシンプルな HTML を直書き
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><title>新規登録</title></head>
<body>
<h1>新規登録</h1>
<form method="POST" action="/register">
  <label>名前: <input type="text" name="name" required></label><br>
  <label>メールアドレス: <input type="email" name="email" required></label><br>
  <label>パスワード: <input type="password" name="password" required></label><br>
  <label>パスワード確認: <input type="password" name="password_confirmation" required></label><br>
  <button type="submit">登録</button>
</form>
<p><a href="/login">ログインはこちら</a></p>
</body>
</html>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, nil)
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
		writeError(w, http.StatusUnprocessableEntity, "name, email, password, password_confirmation は必須です")
		return
	}

	// バリデーション: パスワード一致確認
	if password != passwordConfirmation {
		writeError(w, http.StatusUnprocessableEntity, "パスワードと確認用パスワードが一致しません")
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
		writeError(w, http.StatusInternalServerError, "ユーザーの作成に失敗しました: "+err.Error())
		return
	}

	if err := middleware.SetUserInSession(r, w, h.store, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}
