package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
)

type PushPayload struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	ID    int64                  `json:"id,omitempty"`
	Type  string                 `json:"type,omitempty"`
	URL   string                 `json:"url,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

type PushService struct {
	repo       repository.PushSubscriptionRepositoryInterface
	publicKey  string
	privateKey string
	subject    string
	httpClient webpush.HTTPClient
}

func NewPushServiceFromEnv(repo repository.PushSubscriptionRepositoryInterface) *PushService {
	return &PushService{
		repo:       repo,
		publicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		privateKey: os.Getenv("VAPID_PRIVATE_KEY"),
		subject:    os.Getenv("VAPID_SUBJECT"),
	}
}

func (s *PushService) PublicKey() string {
	return s.publicKey
}

func (s *PushService) Enabled() bool {
	return s.publicKey != "" && s.privateKey != "" && s.subject != ""
}

func (s *PushService) Subscribe(userID int64, endpoint, p256dh, auth string, userAgent *string) error {
	if endpoint == "" || p256dh == "" || auth == "" {
		return fmt.Errorf("endpoint, p256dh, auth are required")
	}
	return s.repo.Upsert(&model.PushSubscription{
		UserID:    userID,
		Endpoint:  endpoint,
		P256dh:    p256dh,
		Auth:      auth,
		UserAgent: userAgent,
	})
}

func (s *PushService) Unsubscribe(userID int64, endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	return s.repo.Delete(userID, endpoint)
}

func (s *PushService) SendToUser(ctx context.Context, userID int64, payload PushPayload) error {
	if !s.Enabled() {
		return nil
	}
	subscriptions, err := s.repo.FindByUser(userID)
	if err != nil {
		return err
	}
	if len(subscriptions) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json.Marshal push payload: %w", err)
	}
	for _, sub := range subscriptions {
		sub := sub
		status, err := s.send(ctx, &sub, body)
		if status == http.StatusNotFound || status == http.StatusGone {
			if delErr := s.repo.Delete(userID, sub.Endpoint); delErr != nil {
				slog.Warn("expired push subscription delete failed", "user_id", userID, "endpoint", sub.Endpoint, "error", delErr)
			}
		}
		if err != nil {
			slog.Warn("web push send failed", "user_id", userID, "endpoint", sub.Endpoint, "error", err)
		}
	}
	return nil
}

func (s *PushService) send(ctx context.Context, sub *model.PushSubscription, payload []byte) (int, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}, &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             int((24 * time.Hour).Seconds()),
		Urgency:         webpush.UrgencyNormal,
		HTTPClient:      s.httpClient,
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("push endpoint returned status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
