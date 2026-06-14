-- recording_schedules に定期録音カラムを追加
ALTER TABLE `recording_schedules`
  ADD COLUMN `is_recurring`     TINYINT(1)   NOT NULL DEFAULT 0    COMMENT '定期録音フラグ' AFTER `error_message`,
  ADD COLUMN `recurrence_type`  VARCHAR(20)  NULL DEFAULT NULL     COMMENT '繰り返し種別 (weekly)' AFTER `is_recurring`,
  ADD COLUMN `parent_schedule_id` BIGINT UNSIGNED NULL DEFAULT NULL COMMENT '親スケジュールID（定期録音の次回生成元）' AFTER `recurrence_type`,
  ADD KEY `idx_recording_schedules_parent` (`parent_schedule_id`);
