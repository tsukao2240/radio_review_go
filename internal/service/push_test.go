package service

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

type fakePushRepo struct {
	subscriptions []model.PushSubscription
	deleted       []string
}

func (r *fakePushRepo) FindByUser(userID int64) ([]model.PushSubscription, error) {
	return r.subscriptions, nil
}

func (r *fakePushRepo) Upsert(subscription *model.PushSubscription) error {
	r.subscriptions = append(r.subscriptions, *subscription)
	return nil
}

func (r *fakePushRepo) Delete(userID int64, endpoint string) error {
	r.deleted = append(r.deleted, endpoint)
	return nil
}

type fakePushHTTPClient struct {
	statusCode int
	calls      int
}

func (c *fakePushHTTPClient) Do(*http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{
		StatusCode: c.statusCode,
		Body:       io.NopCloser(http.NoBody),
	}, nil
}

func TestPushServiceSendToUserSuccess(t *testing.T) {
	repo := &fakePushRepo{subscriptions: []model.PushSubscription{testPushSubscription()}}
	client := &fakePushHTTPClient{statusCode: http.StatusCreated}
	svc := &PushService{
		repo:       repo,
		publicKey:  "test-public",
		privateKey: "test-private",
		subject:    "mailto:test@example.com",
		httpClient: client,
	}

	if err := svc.SendToUser(context.Background(), 7, PushPayload{Title: "録音完了", Body: "done"}); err != nil {
		t.Fatalf("SendToUser: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", repo.deleted)
	}
}

func TestPushServiceDisabled(t *testing.T) {
	repo := &fakePushRepo{subscriptions: []model.PushSubscription{testPushSubscription()}}
	svc := &PushService{repo: repo}
	if svc.Enabled() {
		t.Fatal("Enabled = true, want false")
	}
	if err := svc.Subscribe(7, "https://push.example/sub", "p", "a", nil); err == nil {
		t.Fatal("expected Subscribe error when disabled")
	}
	if err := svc.SendToUser(context.Background(), 7, PushPayload{Title: "録音完了"}); err != nil {
		t.Fatalf("SendToUser disabled: %v", err)
	}
}

func TestPushServiceSendToUserDeletesGoneSubscription(t *testing.T) {
	sub := testPushSubscription()
	repo := &fakePushRepo{subscriptions: []model.PushSubscription{sub}}
	client := &fakePushHTTPClient{statusCode: http.StatusGone}
	svc := &PushService{
		repo:       repo,
		publicKey:  "test-public",
		privateKey: "test-private",
		subject:    "mailto:test@example.com",
		httpClient: client,
	}

	if err := svc.SendToUser(context.Background(), 7, PushPayload{Title: "録音失敗", Body: "failed"}); err != nil {
		t.Fatalf("SendToUser: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != sub.Endpoint {
		t.Fatalf("deleted = %#v, want %q", repo.deleted, sub.Endpoint)
	}
}

func testPushSubscription() model.PushSubscription {
	return model.PushSubscription{
		UserID:   7,
		Endpoint: "https://updates.push.services.mozilla.com/wpush/v2/gAAAAA",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
}
