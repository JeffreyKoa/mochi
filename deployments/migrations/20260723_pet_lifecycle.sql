-- Pet lifecycle: real 1:1 calendar age, species, SKU fields

ALTER TABLE pets
    ADD COLUMN sku_id VARCHAR(64) NOT NULL DEFAULT '' AFTER personality_json,
    ADD COLUMN species VARCHAR(16) NOT NULL DEFAULT 'cat' AFTER sku_id,
    ADD COLUMN breed VARCHAR(32) NOT NULL DEFAULT '' AFTER species,
    ADD COLUMN born_at DATETIME(3) NULL AFTER breed,
    ADD COLUMN max_age_years FLOAT NOT NULL DEFAULT 18 AFTER born_at,
    ADD COLUMN life_stage VARCHAR(16) NOT NULL DEFAULT 'newborn' AFTER max_age_years,
    ADD COLUMN is_alive TINYINT(1) NOT NULL DEFAULT 1 AFTER life_stage;

UPDATE pets
SET born_at = created_at
WHERE born_at IS NULL;

UPDATE pets
SET species = 'cat'
WHERE species = '' OR species IS NULL;

UPDATE pets
SET max_age_years = 18
WHERE max_age_years = 0 OR max_age_years IS NULL;

UPDATE pets
SET life_stage = 'newborn', is_alive = 1
WHERE life_stage = '' OR life_stage IS NULL;
