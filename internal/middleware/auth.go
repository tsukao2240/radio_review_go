package middleware

import (
	"context"
	"net/http"

	"github.com/gorilla/sessions"
)

type contextKey string

const UserContextKey contextKey = "user"

const (
	sessionName    = "radio_review_session"
	sessionUserKey = "user_id"
)

// RequireAuth は未認証リクエストを /login にリダイレクトするミドルウェア。
// セッションに user_id が存在する場合はコンテキストにセットして次のハンドラーへ進む。
func RequireAuth(store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, sessionName)
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			userIDVal, ok := session.Values[sessionUserKey]
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			userID, ok := userIDVal.(int64)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID はコンテキストからユーザーIDを取得するヘルパー。
// 認証済みであれば (userID, true) を、未認証であれば (0, false) を返す。
func GetUserID(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserContextKey).(int64)
	return userID, ok
}

// SetUserInSession はログイン成功時にセッションへユーザーIDを保存する。
func SetUserInSession(r *http.Request, w http.ResponseWriter, store sessions.Store, userID int64) error {
	session, err := store.Get(r, sessionName)
	if err != nil {
		return err
	}

	session.Values[sessionUserKey] = userID
	return session.Save(r, w)
}

// ClearSession はログアウト時にセッションをクリアする。
// MaxAge を -1 にすることでクッキーを即時削除させる。
func ClearSession(r *http.Request, w http.ResponseWriter, store sessions.Store) error {
	session, err := store.Get(r, sessionName)
	if err != nil {
		return err
	}

	session.Values = make(map[interface{}]interface{})
	session.Options.MaxAge = -1
	return session.Save(r, w)
}
