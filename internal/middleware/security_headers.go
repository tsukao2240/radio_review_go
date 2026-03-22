package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
)

type nonceContextKey string

const NonceContextKey nonceContextKey = "csp_nonce"

// generateNonce は crypto/rand を使って16バイトのランダムなBase64文字列を生成する。
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SecurityHeaders はセキュリティ関連のHTTPレスポンスヘッダーを設定するミドルウェア。
// CSP の script-src には毎リクエストごとに生成したランダムなnonceを付与し、
// そのnonce値をコンテキストに保存することでテンプレート側から参照できる。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := generateNonce()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		csp := fmt.Sprintf(
			"default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'",
			nonce,
		)

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		ctx := context.WithValue(r.Context(), NonceContextKey, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetNonce はコンテキストからCSP nonceを取得するヘルパー。
// テンプレートで <script nonce="{{ nonce }}"> のように使う。
func GetNonce(ctx context.Context) string {
	nonce, _ := ctx.Value(NonceContextKey).(string)
	return nonce
}
