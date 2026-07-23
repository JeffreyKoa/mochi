-- Mochi Life Agent Phase1
-- Applied via MCP to `mochi` database on 2026-07-23

ALTER TABLE memories
  MODIFY COLUMN type VARCHAR(16) NOT NULL DEFAULT 'long';

CREATE TABLE IF NOT EXISTS bond_profiles (
  pet_id BIGINT UNSIGNED NOT NULL,
  rapport_level TINYINT UNSIGNED NOT NULL DEFAULT 20,
  trust_level TINYINT UNSIGNED NOT NULL DEFAULT 15,
  shared_topics JSON NULL,
  nicknames JSON NULL,
  inside_jokes JSON NULL,
  last_mood_tag VARCHAR(32) NOT NULL DEFAULT '',
  last_intent VARCHAR(32) NOT NULL DEFAULT '',
  last_mood_at DATETIME(3) NULL,
  total_turns INT NOT NULL DEFAULT 0,
  last_chat_day VARCHAR(10) NOT NULL DEFAULT '',
  streak_days INT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NULL,
  PRIMARY KEY (pet_id),
  CONSTRAINT fk_bond_profiles_pet FOREIGN KEY (pet_id) REFERENCES pets(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO bond_profiles (pet_id, rapport_level, trust_level, shared_topics, nicknames, inside_jokes, updated_at)
SELECT p.id, 20, 15, '[]', '{}', '[]', NOW(3)
FROM pets p
LEFT JOIN bond_profiles b ON b.pet_id = p.id
WHERE b.pet_id IS NULL;
