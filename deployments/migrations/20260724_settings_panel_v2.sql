-- Settings Panel V2: pet gender + user preferences

ALTER TABLE pets
    ADD COLUMN gender VARCHAR(8) NOT NULL DEFAULT 'female' AFTER breed;

UPDATE pets SET gender = 'female' WHERE gender = '' OR gender IS NULL;

ALTER TABLE users
    ADD COLUMN quiet_hours_start INT NOT NULL DEFAULT 23 AFTER proactive_enabled,
    ADD COLUMN quiet_hours_end INT NOT NULL DEFAULT 8 AFTER quiet_hours_start,
    ADD COLUMN morning_greeting TINYINT(1) NOT NULL DEFAULT 1 AFTER quiet_hours_end,
    ADD COLUMN reminder_voice TINYINT(1) NOT NULL DEFAULT 1 AFTER morning_greeting,
    ADD COLUMN follow_up_enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER reminder_voice,
    ADD COLUMN voice_reply_default TINYINT(1) NOT NULL DEFAULT 1 AFTER follow_up_enabled,
    ADD COLUMN stt_mode VARCHAR(16) NOT NULL DEFAULT 'auto' AFTER voice_reply_default,
    ADD COLUMN learning_prefs_json JSON NULL AFTER stt_mode;
