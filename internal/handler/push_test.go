package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePushService struct {
	publicKey  string
	enabled    bool
	subscribed struct {
		userID   int64
		endpoint string
		p256dh   string
		auth     string
	}
	unsubscribed struct {
		userID   int64
		endpoint string
	}
}

func (s *fakePushService) PublicKey() string { return s.publicKey }
func (s *fakePushService) Enabled() bool     { return s.enabled }
func (s *fakePushService) Subscribe(userID int64, endpoint, p256dh, auth string, userAgent *string) error {
	s.subscribed.userID = userID
	s.subscribed.endpoint = endpoint
	s.subscribed.p256dh = p256dh
	s.subscribed.auth = auth
	return nil
}
func (s *fakePushService) Unsubscribe(userID int64, endpoint string) error {
	s.unsubscribed.userID = userID
	s.unsubscribed.endpoint = endpoint
	return nil
}

func TestPushHandlerVAPIDPublicKey(t *testing.T) {
	h := NewPushHandler(&fakePushService{publicKey: "public", enabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/push/vapid-public-key", nil)
	rr := httptest.NewRecorder()

	h.VAPIDPublicKey(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["public_key"] != "public" || resp["enabled"] != true {
		t.Fatalf("response = %#v", resp)
	}
}

func TestPushHandlerSubscribe(t *testing.T) {
	svc := &fakePushService{}
	h := NewPushHandler(svc)
	body := []byte(`{"endpoint":"https://push.example/sub","keys":{"p256dh":"p","auth":"a"}}`)
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", bytes.NewReader(body)), 7)
	rr := httptest.NewRecorder()

	h.Subscribe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if svc.subscribed.userID != 7 || svc.subscribed.endpoint != "https://push.example/sub" || svc.subscribed.p256dh != "p" || svc.subscribed.auth != "a" {
		t.Fatalf("subscribed = %#v", svc.subscribed)
	}
}

func TestPushHandlerUnsubscribe(t *testing.T) {
	svc := &fakePushService{}
	h := NewPushHandler(svc)
	body := []byte(`{"endpoint":"https://push.example/sub"}`)
	req := withUserID(httptest.NewRequest(http.MethodDelete, "/api/push/subscribe", bytes.NewReader(body)), 7)
	rr := httptest.NewRecorder()

	h.Unsubscribe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if svc.unsubscribed.userID != 7 || svc.unsubscribed.endpoint != "https://push.example/sub" {
		t.Fatalf("unsubscribed = %#v", svc.unsubscribed)
	}
}
