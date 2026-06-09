package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type PushSubscriptionRepository struct {
	db *sqlx.DB
}

func NewPushSubscriptionRepository(db *sqlx.DB) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{db: db}
}

func (r *PushSubscriptionRepository) FindByUser(userID int64) ([]model.PushSubscription, error) {
	var subscriptions []model.PushSubscription
	err := r.db.Select(&subscriptions,
		`SELECT * FROM push_subscriptions
		 WHERE user_id = ?
		 ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PushSubscriptionRepository.FindByUser: %w", err)
	}
	return subscriptions, nil
}

func (r *PushSubscriptionRepository) Upsert(subscription *model.PushSubscription) error {
	subscription.EndpointHash = EndpointHash(subscription.Endpoint)
	_, err := r.db.Exec(
		`INSERT INTO push_subscriptions (user_id, endpoint, endpoint_hash, p256dh, auth, user_agent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		 ON DUPLICATE KEY UPDATE
		   endpoint = VALUES(endpoint),
		   p256dh = VALUES(p256dh),
		   auth = VALUES(auth),
		   user_agent = VALUES(user_agent),
		   updated_at = NOW()`,
		subscription.UserID,
		subscription.Endpoint,
		subscription.EndpointHash,
		subscription.P256dh,
		subscription.Auth,
		subscription.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("repository.PushSubscriptionRepository.Upsert: %w", err)
	}
	return nil
}

func (r *PushSubscriptionRepository) Delete(userID int64, endpoint string) error {
	_, err := r.db.Exec(
		"DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint_hash = ?",
		userID,
		EndpointHash(endpoint),
	)
	if err != nil {
		return fmt.Errorf("repository.PushSubscriptionRepository.Delete: %w", err)
	}
	return nil
}

func EndpointHash(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}
