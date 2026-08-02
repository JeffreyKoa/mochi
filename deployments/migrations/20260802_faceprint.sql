-- Owner faceprint enrollment (InsightFace-style embedding, per user)

CREATE TABLE IF NOT EXISTS faceprints (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    embedding TEXT NOT NULL COMMENT 'JSON array of float32 face embedding',
    dim INT NOT NULL DEFAULT 512,
    samples INT NOT NULL DEFAULT 0 COMMENT 'number of enrollment snapshots averaged',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_faceprints_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
