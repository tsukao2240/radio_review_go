-- 001_initial.sql
-- MySQL 8.0 向け初期スキーマ
-- PHP/Laravel のマイグレーションを1ファイルにまとめたもの

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- --------------------------------------------------------
-- users
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `users` (
  `id`                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name`              VARCHAR(255)    NOT NULL,
  `email`             VARCHAR(255)    NOT NULL,
  `email_verified_at` TIMESTAMP       NULL DEFAULT NULL,
  `password`          VARCHAR(255)    NOT NULL,
  `remember_token`    VARCHAR(100)    NULL DEFAULT NULL,
  `created_at`        TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`        TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `users_email_unique` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- password_resets
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `password_resets` (
  `email`      VARCHAR(255) NOT NULL,
  `token`      VARCHAR(255) NOT NULL,
  `created_at` TIMESTAMP    NULL DEFAULT NULL,
  KEY `password_resets_email_index` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- radio_programs
-- timestamps なし（model.RadioProgram の $timestamps = false に対応）
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `radio_programs` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `station_id` VARCHAR(255)    NOT NULL,
  `title`      VARCHAR(255)    NOT NULL,
  `cast`       TEXT            NULL DEFAULT NULL,
  `start`      VARCHAR(255)    NOT NULL,
  `end`        VARCHAR(255)    NOT NULL,
  `info`       TEXT            NULL DEFAULT NULL,
  `url`        TEXT            NULL DEFAULT NULL,
  `image`      TEXT            NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_radio_programs_title`         (`title`(191)),
  KEY `idx_radio_programs_station_id`    (`station_id`),
  KEY `idx_radio_programs_station_title` (`station_id`, `title`(191))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- posts
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `posts` (
  `id`             BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
  `user_id`        BIGINT UNSIGNED  NOT NULL,
  `program_id`     BIGINT UNSIGNED  NOT NULL,
  `program_title`  VARCHAR(255)     NOT NULL,
  `station_id`     VARCHAR(255)     NULL DEFAULT NULL,
  `title`          VARCHAR(255)     NOT NULL,
  `body`           TEXT             NOT NULL,
  `rating`         DECIMAL(2,1)     NOT NULL DEFAULT 3.0 COMMENT '評価（1.0-5.0）',
  `likes_count`    INT UNSIGNED     NOT NULL DEFAULT 0   COMMENT 'いいね数',
  `comments_count` INT UNSIGNED     NOT NULL DEFAULT 0   COMMENT 'コメント数',
  `created_at`     TIMESTAMP        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`     TIMESTAMP        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `posts_user_id_index`    (`user_id`),
  KEY `posts_program_id_index` (`program_id`),
  KEY `posts_station_id_index` (`station_id`),
  KEY `posts_created_at_index` (`created_at`),
  KEY `posts_rating_index`     (`rating`),
  KEY `posts_likes_count_index`(`likes_count`),
  CONSTRAINT `posts_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- post_tags
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `post_tags` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name`          VARCHAR(50)     NOT NULL COMMENT 'タグ名',
  `display_order` INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '表示順',
  `created_at`    TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `post_tags_name_unique` (`name`),
  KEY `post_tags_name_index` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- post_post_tag  （posts と post_tags の中間テーブル）
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `post_post_tag` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `post_id`     BIGINT UNSIGNED NOT NULL,
  `post_tag_id` BIGINT UNSIGNED NOT NULL,
  `created_at`  TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `post_post_tag_post_id_tag_id_unique` (`post_id`, `post_tag_id`),
  KEY `post_post_tag_post_tag_id_index` (`post_tag_id`),
  CONSTRAINT `post_post_tag_post_id_foreign`     FOREIGN KEY (`post_id`)     REFERENCES `posts`     (`id`) ON DELETE CASCADE,
  CONSTRAINT `post_post_tag_post_tag_id_foreign` FOREIGN KEY (`post_tag_id`) REFERENCES `post_tags` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- post_likes
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `post_likes` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `post_id`    BIGINT UNSIGNED NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `created_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `post_likes_post_id_user_id_unique` (`post_id`, `user_id`),
  KEY `post_likes_post_id_index`    (`post_id`),
  KEY `post_likes_user_id_index`    (`user_id`),
  KEY `post_likes_created_at_index` (`created_at`),
  CONSTRAINT `post_likes_post_id_foreign` FOREIGN KEY (`post_id`) REFERENCES `posts` (`id`) ON DELETE CASCADE,
  CONSTRAINT `post_likes_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- post_comments
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `post_comments` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `post_id`    BIGINT UNSIGNED NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `body`       TEXT            NOT NULL COMMENT 'コメント内容（最大1000文字）',
  `created_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `post_comments_post_id_index`    (`post_id`),
  KEY `post_comments_user_id_index`    (`user_id`),
  KEY `post_comments_created_at_index` (`created_at`),
  CONSTRAINT `post_comments_post_id_foreign` FOREIGN KEY (`post_id`) REFERENCES `posts` (`id`) ON DELETE CASCADE,
  CONSTRAINT `post_comments_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- favorite_programs
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `favorite_programs` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`       BIGINT UNSIGNED NOT NULL,
  `station_id`    VARCHAR(50)     NOT NULL,
  `program_title` VARCHAR(255)    NOT NULL,
  `broadcast_day` TINYINT         NULL DEFAULT NULL COMMENT '0=月, 1=火, 2=水, 3=木, 4=金, 5=土, 6=日',
  `created_at`    TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `fav_programs_unique` (`user_id`, `station_id`, `program_title`, `broadcast_day`),
  KEY `idx_favorite_programs_user_id` (`user_id`),
  CONSTRAINT `favorite_programs_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- recording_schedules
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `recording_schedules` (
  `id`                   BIGINT UNSIGNED                                               NOT NULL AUTO_INCREMENT,
  `user_id`              BIGINT UNSIGNED                                               NOT NULL,
  `station_id`           VARCHAR(255)                                                  NOT NULL COMMENT '放送局ID（例: TBS, QRR）',
  `program_title`        VARCHAR(255)                                                  NOT NULL COMMENT '番組名',
  `scheduled_start_time` DATETIME                                                      NOT NULL COMMENT '予約開始時刻',
  `scheduled_end_time`   DATETIME                                                      NOT NULL COMMENT '予約終了時刻',
  `status`               ENUM('pending','recording','completed','failed','cancelled')  NOT NULL DEFAULT 'pending',
  `recording_id`         VARCHAR(255)                                                  NULL DEFAULT NULL COMMENT '録音ID（録音開始後に設定）',
  `error_message`        TEXT                                                          NULL DEFAULT NULL COMMENT 'エラーメッセージ',
  `created_at`           TIMESTAMP                                                     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`           TIMESTAMP                                                     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_recording_schedules_user_id`    (`user_id`),
  KEY `idx_recording_schedules_status`     (`status`),
  KEY `idx_recording_schedules_start_time` (`scheduled_start_time`),
  KEY `recording_schedules_user_status`    (`user_id`, `status`),
  CONSTRAINT `recording_schedules_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- notifications
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `notifications` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `type`       VARCHAR(255)    NOT NULL COMMENT 'recording_start / recording_complete / favorite_broadcast',
  `title`      VARCHAR(255)    NOT NULL,
  `message`    TEXT            NOT NULL,
  `data`       JSON            NULL DEFAULT NULL COMMENT '追加データ（録音ID、番組情報など）',
  `is_read`    TINYINT(1)      NOT NULL DEFAULT 0,
  `read_at`    TIMESTAMP       NULL DEFAULT NULL,
  `created_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `notifications_user_id_index`         (`user_id`),
  KEY `notifications_user_is_read_index`    (`user_id`, `is_read`),
  KEY `notifications_created_at_index`      (`created_at`),
  CONSTRAINT `notifications_user_id_foreign` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
-- failed_jobs  （Laravel キュー互換。Go では直接使わないが念のため）
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `failed_jobs` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `connection` TEXT            NOT NULL,
  `queue`      TEXT            NOT NULL,
  `payload`    LONGTEXT        NOT NULL,
  `exception`  LONGTEXT        NOT NULL,
  `failed_at`  TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;
