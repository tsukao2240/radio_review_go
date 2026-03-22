// Package radiko はRadiko APIとの認証・通信を担当する。
// 注意: InsecureSkipVerify: true はRadiko API互換性のため意図的に設定している。
package radiko

import (
	"context"
	"crypto/tls"
	"net/http"

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

// GetAuthToken は未実装（フェーズ2で実装）。
func (c *Client) GetAuthToken(ctx context.Context, areaID string) (string, error) {
	panic("not implemented: will be implemented in Phase 2")
}
