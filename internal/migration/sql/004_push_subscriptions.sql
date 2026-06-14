-- Web Push 購読情報

CREATE TABLE IF NOT EXISTS `push_subscriptions` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `endpoint`   TEXT            NOT NULL,
  `endpoint_hash` CHAR(64)     NOT NULL,
  `p256dh`     VARCHAR(255)    NOT NULL,
  `auth`       VARCHAR(255)    NOT NULL,
  `user_agent` TEXT            NULL DEFAULT NULL,
  `created_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `push_subscriptions_user_endpoint_hash_unique` (`user_id`, `endpoint_hash`),
  KEY `push_subscriptions_user_id_index` (`user_id`),
  CONSTRAINT `push_subscriptions_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
