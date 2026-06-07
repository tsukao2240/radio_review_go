// Package radiko はRadiko APIとの認証・通信を担当する。
// 注意: InsecureSkipVerify: true はRadiko API互換性のため意図的に設定している。
package radiko

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/redis/go-redis/v9"
)

// ClientInterface はRadiko認証クライアントのインターフェース。
type ClientInterface interface {
	// GetAuthToken は指定エリアの認証トークンを返す。
	// Redisに55分キャッシュ（キー: radiko_auth_token_{areaId}）。
	GetAuthToken(ctx context.Context, areaID string) (string, error)
}

// Client はRadiko API認証を行うクライアント。
type Client struct {
	httpClient *http.Client
	redis      *redis.Client
	keyPath    string // storage/keys/radiko_auth_key.txt のパス
}

// NewClient は新しいRadikoクライアントを返す。
// InsecureSkipVerify: true はRadiko API互換性のため意図的。
func NewClient(redisClient *redis.Client, keyPath string) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Radiko APIがSSL検証を通らないため意図的
		},
	}
	return &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      redisClient,
		keyPath:    keyPath,
	}
}

// GetAuthToken はRadiko認証トークンを返す。
// まずRedisキャッシュを確認し、なければauth1 → 部分キー生成 → auth2 の2ステップ認証を実行する。
// 取得したトークンは radiko_auth_token_{areaId} キーで55分間キャッシュする。
func (c *Client) GetAuthToken(ctx context.Context, areaID string) (string, error) {
	if strings.TrimSpace(areaID) == "" {
		areaID = "JP13"
	}
	cacheKey := "radiko_auth_token_" + areaID

	// Redisキャッシュを確認
	cached, err := c.redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		return cached, nil
	}

	// ステップ1: auth1 でトークンとキー情報を取得
	req1, err := http.NewRequest(http.MethodGet, "https://radiko.jp/v2/api/auth1", nil)
	if err != nil {
		return "", fmt.Errorf("radiko auth1: リクエスト生成エラー: %w", err)
	}
	req1 = req1.WithContext(ctx)
	appVersion := "8.2.4"
	userID := randomUserID()
	device := "34.GooglePixel6"
	req1.Header.Set("X-Radiko-App", "aSmartPhone8")
	req1.Header.Set("X-Radiko-App-Version", appVersion)
	req1.Header.Set("X-Radiko-Device", device)
	req1.Header.Set("X-Radiko-User", userID)

	resp1, err := c.httpClient.Do(req1)
	if err != nil {
		return "", fmt.Errorf("radiko auth1: HTTPリクエストエラー: %w", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		return "", fmt.Errorf("radiko auth1: HTTPステータスエラー: %d", resp1.StatusCode)
	}

	authToken := resp1.Header.Get("X-Radiko-AuthToken")
	keyLengthStr := resp1.Header.Get("X-Radiko-KeyLength")
	keyOffsetStr := resp1.Header.Get("X-Radiko-KeyOffset")

	if authToken == "" {
		return "", fmt.Errorf("radiko auth1: X-Radiko-AuthToken ヘッダーが空")
	}
	if keyLengthStr == "" {
		return "", fmt.Errorf("radiko auth1: X-Radiko-KeyLength ヘッダーが空")
	}
	if keyOffsetStr == "" {
		return "", fmt.Errorf("radiko auth1: X-Radiko-KeyOffset ヘッダーが空")
	}

	keyLength, err := strconv.Atoi(strings.TrimSpace(keyLengthStr))
	if err != nil {
		return "", fmt.Errorf("radiko auth1: X-Radiko-KeyLength パースエラー: %w", err)
	}
	keyOffset, err := strconv.Atoi(strings.TrimSpace(keyOffsetStr))
	if err != nil {
		return "", fmt.Errorf("radiko auth1: X-Radiko-KeyOffset パースエラー: %w", err)
	}

	// ステップ2: 部分キー生成
	// 認証キーファイル（Base64エンコード済み）を読み込んでデコード
	keyB64, err := os.ReadFile(c.keyPath)
	if err != nil {
		return "", fmt.Errorf("radiko 部分キー生成: 認証キーファイル読み込みエラー (%s): %w", c.keyPath, err)
	}

	authKeyBytes, err := decodeRadikoAuthKeyBase64(keyB64)
	if err != nil {
		return "", fmt.Errorf("radiko 部分キー生成: Base64デコードエラー: %w", err)
	}

	if len(authKeyBytes) < keyOffset+keyLength {
		return "", fmt.Errorf("radiko 部分キー生成: オフセット範囲外 (keyLen=%d, offset=%d, extractLen=%d)",
			len(authKeyBytes), keyOffset, keyLength)
	}

	extracted := authKeyBytes[keyOffset : keyOffset+keyLength]
	partialKey := base64.StdEncoding.EncodeToString(extracted)

	// ステップ3: auth2 で認証完了
	req2, err := http.NewRequest(http.MethodGet, "https://radiko.jp/v2/api/auth2", nil)
	if err != nil {
		return "", fmt.Errorf("radiko auth2: リクエスト生成エラー: %w", err)
	}
	req2 = req2.WithContext(ctx)
	req2.Header.Set("X-Radiko-App", "aSmartPhone8")
	req2.Header.Set("X-Radiko-App-Version", appVersion)
	req2.Header.Set("X-Radiko-Device", device)
	req2.Header.Set("X-Radiko-User", userID)
	req2.Header.Set("X-Radiko-AuthToken", authToken)
	req2.Header.Set("X-Radiko-PartialKey", partialKey)
	req2.Header.Set("X-Radiko-Location", generateGPSLocation(areaID))

	resp2, err := c.httpClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("radiko auth2: HTTPリクエストエラー: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return "", fmt.Errorf("radiko auth2: HTTPステータスエラー: %d", resp2.StatusCode)
	}

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", fmt.Errorf("radiko auth2: レスポンスボディ読み込みエラー: %w", err)
	}

	// レスポンスボディ形式: "JP13,<areaId>,..." → カンマ区切りの2番目の要素がareaID
	auth2Body := strings.TrimSpace(string(body2))
	parts := strings.Split(auth2Body, ",")
	if len(parts) < 2 {
		return "", fmt.Errorf("radiko auth2: レスポンスボディ形式が不正: %q", auth2Body)
	}
	// areaIDが引数と一致しない場合でも取得したトークンを利用する（引数areaIDでキャッシュ）

	// Redisキャッシュに55分（3300秒）保存
	if err := c.redis.Set(ctx, cacheKey, authToken, 3300*time.Second).Err(); err != nil {
		// キャッシュ保存失敗は致命的ではないのでエラーをログには残さず処理を続行
		_ = err
	}

	return authToken, nil
}

func randomUserID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%032d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func decodeRadikoAuthKeyBase64(src []byte) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, string(src))
	enc := base64.StdEncoding
	if len(cleaned)%4 != 0 {
		enc = base64.RawStdEncoding
	}
	return enc.DecodeString(cleaned)
}
