package radiko

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

func TestNewClient(t *testing.T) {
	_, rdb := newTestMiniRedis(t)
	c := NewClient(rdb, "testpath")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.keyPath != "testpath" {
		t.Errorf("keyPath: got %q, want %q", c.keyPath, "testpath")
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestGetAuthToken_CacheHit(t *testing.T) {
	_, rdb := newTestMiniRedis(t)
	c := NewClient(rdb, "")

	ctx := context.Background()
	rdb.Set(ctx, "radiko_auth_token_JP13", "cached-token", 0)

	token, err := c.GetAuthToken(ctx, "JP13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "cached-token" {
		t.Errorf("got %q, want %q", token, "cached-token")
	}
}

// makeAuthKey creates a temp auth key file with enough bytes for the given offset+length.
func makeAuthKey(t *testing.T, size int) string {
	t.Helper()
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = byte(i % 256)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "radiko_auth_key.txt")
	if err := os.WriteFile(p, []byte(encoded), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

func TestGetAuthToken_FullFlow(t *testing.T) {
	keyPath := makeAuthKey(t, 256)

	// The partial key extracted will be bytes[16:32] from our raw key.
	raw := make([]byte, 256)
	for i := range raw {
		raw[i] = byte(i % 256)
	}
	partialKey := base64.StdEncoding.EncodeToString(raw[16:32])

	var srvAddr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/api/auth1":
			w.Header().Set("X-Radiko-AuthToken", "test-auth-token")
			w.Header().Set("X-Radiko-KeyLength", "16")
			w.Header().Set("X-Radiko-KeyOffset", "16")
			w.WriteHeader(http.StatusOK)
		case "/v2/api/auth2":
			if r.Header.Get("X-Radiko-AuthToken") != "test-auth-token" {
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-Radiko-PartialKey") != partialKey {
				http.Error(w, "bad partial key", http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, "JP13,JP13,1,1")
		default:
			http.NotFound(w, r)
		}
	}))
	srvAddr = srv.Listener.Addr().String()
	_ = srvAddr
	defer srv.Close()

	// Redirect all HTTPS radiko.jp calls to our test server.
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    keyPath,
	}

	token, err := c.GetAuthToken(context.Background(), "JP13")
	if err != nil {
		t.Fatalf("GetAuthToken: %v", err)
	}
	if token != "test-auth-token" {
		t.Errorf("got %q, want %q", token, "test-auth-token")
	}

	// Verify it's cached.
	cached, err := rdb.Get(context.Background(), "radiko_auth_token_JP13").Result()
	if err != nil {
		t.Fatalf("redis Get: %v", err)
	}
	if cached != "test-auth-token" {
		t.Errorf("cached: got %q, want %q", cached, "test-auth-token")
	}
}

func TestGetAuthToken_Auth1NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    "",
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for auth1 500, got nil")
	}
}

func TestGetAuthToken_MissingAuthToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 200 but no X-Radiko-AuthToken header.
		w.Header().Set("X-Radiko-KeyLength", "16")
		w.Header().Set("X-Radiko-KeyOffset", "16")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    "",
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for missing auth token header, got nil")
	}
}

func TestGetAuthToken_MissingKeyLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Radiko-AuthToken", "tok")
		// Missing X-Radiko-KeyLength
		w.Header().Set("X-Radiko-KeyOffset", "16")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    "",
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for missing key length header, got nil")
	}
}

func TestGetAuthToken_MissingKeyOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Radiko-AuthToken", "tok")
		w.Header().Set("X-Radiko-KeyLength", "16")
		// Missing X-Radiko-KeyOffset
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    "",
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for missing key offset header, got nil")
	}
}

func TestGetAuthToken_KeyFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Radiko-AuthToken", "tok")
		w.Header().Set("X-Radiko-KeyLength", "16")
		w.Header().Set("X-Radiko-KeyOffset", "16")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    "/nonexistent/path/key.txt",
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for missing key file, got nil")
	}
}

func TestGetAuthToken_Auth2NonOK(t *testing.T) {
	keyPath := makeAuthKey(t, 256)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/api/auth1":
			w.Header().Set("X-Radiko-AuthToken", "tok")
			w.Header().Set("X-Radiko-KeyLength", "16")
			w.Header().Set("X-Radiko-KeyOffset", "16")
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "auth2 failed", http.StatusForbidden)
		}
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    keyPath,
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for auth2 403, got nil")
	}
}

func TestGetAuthToken_Auth2BadBody(t *testing.T) {
	keyPath := makeAuthKey(t, 256)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/api/auth1":
			w.Header().Set("X-Radiko-AuthToken", "tok")
			w.Header().Set("X-Radiko-KeyLength", "16")
			w.Header().Set("X-Radiko-KeyOffset", "16")
			w.WriteHeader(http.StatusOK)
		default:
			// Return body without comma separator.
			fmt.Fprint(w, "invalid-body")
		}
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		return srv.Client().Transport.RoundTrip(req2)
	})

	_, rdb := newTestMiniRedis(t)
	c := &Client{
		httpClient: &http.Client{Transport: transport},
		redis:      rdb,
		keyPath:    keyPath,
	}

	_, err := c.GetAuthToken(context.Background(), "JP13")
	if err == nil {
		t.Error("expected error for invalid auth2 body, got nil")
	}
}
