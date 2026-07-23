-- Pet SKU catalog (skin = SKU) + subscription orders

CREATE TABLE IF NOT EXISTS pet_skus (
    sku_id              VARCHAR(64) PRIMARY KEY,
    name                VARCHAR(64) NOT NULL,
    species             VARCHAR(16) NOT NULL DEFAULT 'cat',
    breed               VARCHAR(32) NOT NULL DEFAULT '',
    breed_name          VARCHAR(64) NOT NULL DEFAULT '',
    tier                VARCHAR(16) NOT NULL DEFAULT 'standard',
    max_age_years       FLOAT NOT NULL DEFAULT 18,
    price_cny           INT NOT NULL DEFAULT 0,
    tagline             VARCHAR(128) NOT NULL DEFAULT '',
    skin_json           JSON NOT NULL,
    personality_json    JSON NOT NULL,
    sort_order          INT NOT NULL DEFAULT 0,
    enabled             TINYINT(1) NOT NULL DEFAULT 1,
    created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
);

CREATE TABLE IF NOT EXISTS pet_orders (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id             BIGINT UNSIGNED NOT NULL,
    sku_id              VARCHAR(64) NOT NULL,
    status              VARCHAR(16) NOT NULL DEFAULT 'pending',
    personality_json    JSON NULL,
    pet_id              BIGINT UNSIGNED NULL,
    paid_at             DATETIME(3) NULL,
    claimed_at          DATETIME(3) NULL,
    created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_orders_user (user_id, status),
    INDEX idx_orders_sku (sku_id)
);

-- SKU 1: 粉团 — matches current default PetCanvas palette
INSERT INTO pet_skus (sku_id, name, species, breed, breed_name, tier, max_age_years, price_cny, tagline, skin_json, personality_json, sort_order)
VALUES (
    'cat_mochi_pink',
    '跟屁虫·粉团',
    'cat',
    'mochi_pink',
    '粉团猫',
    'standard',
    18,
    68,
    '经典粉色跟屁虫，粘人可爱',
    JSON_OBJECT(
        'shape', 'bunny',
        'colors', JSON_OBJECT(
            'idle', '#ffb3c6',
            'happy', '#ff8fab',
            'sad', '#adb5bd',
            'sleep', '#cdb4db',
            'eat', '#ffd6a5',
            'walk', '#ffcad4',
            'leg', '#ff7aa2',
            'foot', '#d63384',
            'ear_inner', '#ff9eb5'
        )
    ),
    JSON_OBJECT(
        'traits', '粘人、跟屁虫、好奇心强、偶尔小吃醋',
        'speech_style', '短句口语，可爱但不幼稚，会蹭主人'
    ),
    1
)
ON DUPLICATE KEY UPDATE updated_at = CURRENT_TIMESTAMP(3);

-- SKU 2: 紫团 — lavender body, pink accents (user reference image)
INSERT INTO pet_skus (sku_id, name, species, breed, breed_name, tier, max_age_years, price_cny, tagline, skin_json, personality_json, sort_order)
VALUES (
    'cat_mochi_lavender',
    '跟屁虫·紫团',
    'cat',
    'mochi_lavender',
    '紫团猫',
    'standard',
    18,
    68,
    '淡紫软萌，安静黏人',
    JSON_OBJECT(
        'shape', 'bunny',
        'colors', JSON_OBJECT(
            'idle', '#c4b5fd',
            'happy', '#a78bfa',
            'sad', '#9ca3af',
            'sleep', '#ddd6fe',
            'eat', '#e9d5ff',
            'walk', '#c4b5fd',
            'leg', '#f9a8d4',
            'foot', '#ec4899',
            'ear_inner', '#f9a8d4'
        )
    ),
    JSON_OBJECT(
        'traits', '安静、黏人、慢热但忠诚',
        'speech_style', '轻柔短句，偶尔撒娇'
    ),
    2
)
ON DUPLICATE KEY UPDATE updated_at = CURRENT_TIMESTAMP(3);

-- Bind existing first pet (id=1) to default pink SKU
UPDATE pets
SET sku_id = 'cat_mochi_pink',
    species = 'cat',
    breed = 'mochi_pink',
    max_age_years = 18
WHERE id = 1;
