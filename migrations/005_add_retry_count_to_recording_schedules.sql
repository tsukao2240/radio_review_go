-- recording_schedules に録音失敗時の自動再試行回数を追加
ALTER TABLE `recording_schedules`
  ADD COLUMN `retry_count` INT NOT NULL DEFAULT 0 AFTER `error_message`;
