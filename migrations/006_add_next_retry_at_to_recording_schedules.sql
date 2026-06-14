-- recording_schedules に最短再試行時刻を追加
ALTER TABLE `recording_schedules`
  ADD COLUMN `next_retry_at` TIMESTAMP NULL DEFAULT NULL AFTER `retry_count`,
  ADD KEY `idx_recording_schedules_next_retry_at` (`next_retry_at`);
