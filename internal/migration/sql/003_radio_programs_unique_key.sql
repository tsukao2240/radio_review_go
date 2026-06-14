-- radio_programs に (station_id, title, cast) の UNIQUE KEY を追加してUPSERT可能にする
-- 既存データはバッチで再投入するためクリアする

SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE `radio_programs`;
SET FOREIGN_KEY_CHECKS = 1;

-- cast を NOT NULL に変更
ALTER TABLE `radio_programs`
  MODIFY COLUMN `cast` TEXT NOT NULL;

-- (station_id, title prefix, cast prefix) の UNIQUE KEY を追加
ALTER TABLE `radio_programs`
  ADD UNIQUE KEY `uq_radio_programs_station_title_cast` (`station_id`, `title`(191), `cast`(191));
