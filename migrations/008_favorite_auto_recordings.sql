-- 同じユーザー・放送局・放送開始日時の自動録音を一度だけ登録する。
-- 手動予約は NULL のため、既存データの重複は許容する。
ALTER TABLE `recording_schedules`
  MODIFY COLUMN `status` ENUM('pending','recording','completed','failed','cancelled','expired') NOT NULL DEFAULT 'pending',
  ADD COLUMN `auto_recording_key` CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL DEFAULT NULL,
  ADD COLUMN `recording_metadata` JSON NULL DEFAULT NULL,
  ADD UNIQUE KEY `uq_recording_schedules_auto_recording_key` (`auto_recording_key`);
