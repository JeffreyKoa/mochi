-- Gap closure: brief entry approval + user proactive preference

ALTER TABLE user_brief_entries
    ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'approved' AFTER source;

UPDATE user_brief_entries SET status = 'approved' WHERE status IS NULL OR status = '';

ALTER TABLE users
    ADD COLUMN proactive_enabled TINYINT(1) NOT NULL DEFAULT 1;
