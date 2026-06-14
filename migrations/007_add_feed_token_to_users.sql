-- ポッドキャストフィード用のユーザー別秘密トークン
ALTER TABLE `users`
  ADD COLUMN `feed_token` VARCHAR(64) NULL AFTER `remember_token`,
  ADD UNIQUE KEY `users_feed_token_unique` (`feed_token`);

UPDATE `users`
SET `feed_token` = LOWER(HEX(RANDOM_BYTES(32)))
WHERE `feed_token` IS NULL;

ALTER TABLE `users`
  MODIFY COLUMN `feed_token` VARCHAR(64) NOT NULL;
