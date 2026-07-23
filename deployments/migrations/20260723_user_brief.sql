-- UserBrief: curated owner profile for companion prompt (P0 growth engine)

CREATE TABLE IF NOT EXISTS user_briefs (
    pet_id          BIGINT UNSIGNED PRIMARY KEY,
    compiled_text   VARCHAR(1400) NOT NULL DEFAULT '',
    compiled_at     DATETIME(3) NULL,
    char_budget     SMALLINT UNSIGNED NOT NULL DEFAULT 1400,
    updated_at      DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS user_brief_entries (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    pet_id          BIGINT UNSIGNED NOT NULL,
    category        VARCHAR(16) NOT NULL,
    content         VARCHAR(256) NOT NULL,
    importance      FLOAT NOT NULL DEFAULT 0.5,
    source          VARCHAR(16) NOT NULL DEFAULT 'extract',
    created_at      DATETIME(3) NOT NULL,
    updated_at      DATETIME(3) NOT NULL,
    INDEX idx_pet_cat (pet_id, category),
    INDEX idx_pet_imp (pet_id, importance DESC)
);
