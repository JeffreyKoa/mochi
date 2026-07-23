-- Mochi Tools Phase2A: reminders + todos

CREATE TABLE IF NOT EXISTS reminders (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    pet_id          BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,
    title           VARCHAR(256) NOT NULL,
    fire_at         DATETIME(3) NOT NULL,
    repeat_rule     VARCHAR(32) NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    source          VARCHAR(16) NOT NULL DEFAULT 'chat',
    source_msg      VARCHAR(512) NULL,
    fired_at        DATETIME(3) NULL,
    created_at      DATETIME(3) NOT NULL,
    updated_at      DATETIME(3) NOT NULL,
    INDEX idx_pet_fire (pet_id, fire_at, status),
    INDEX idx_user (user_id)
);

CREATE TABLE IF NOT EXISTS todos (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    pet_id          BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,
    title           VARCHAR(256) NOT NULL,
    due_at          DATETIME(3) NULL,
    done            TINYINT(1) NOT NULL DEFAULT 0,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      DATETIME(3) NOT NULL,
    updated_at      DATETIME(3) NOT NULL,
    INDEX idx_pet_done (pet_id, done, created_at DESC)
);
