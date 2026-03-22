package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// entry はIPアドレスごとのリクエスト状態を保持する。
type entry struct {
	mu        sync.Mutex
	count     int
	windowEnd time.Time
}

// RateLimit はエンドポイントごとのインメモリレートリミットミドルウェアを返す。
//
// limit: 許可するリクエスト数
// window: 時間ウィンドウ（例: time.Minute）
//
// 想定レート:
//   - 投稿 (/review/{id} POST, /my/edit POST): RateLimit(10, time.Minute)
//   - 検索 (/search):                          RateLimit(30, time.Minute)
//   - ログイン (/login):                       RateLimit(5, time.Minute)
//
// IPアドレスは X-Forwarded-For ヘッダーを優先し、なければ RemoteAddr を使う。
// 時間ウィンドウが過ぎると自動的にカウントがリセットされる。
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	var ipMap sync.Map // map[string]*entry

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)

			val, _ := ipMap.LoadOrStore(ip, &entry{})
			e := val.(*entry)

			e.mu.Lock()
			now := time.Now()
			if now.After(e.windowEnd) {
				// 新しいウィンドウを開始
				e.count = 0
				e.windowEnd = now.Add(window)
			}
			e.count++
			count := e.count
			e.mu.Unlock()

			if count > limit {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP はリクエストからクライアントIPアドレスを取得する。
// X-Forwarded-For ヘッダーが存在する場合は最初のIPを使い、なければ RemoteAddr を使う。
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For は "client, proxy1, proxy2" 形式
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// SplitHostPort が失敗した場合はそのまま使う
		return r.RemoteAddr
	}
	return ip
}
