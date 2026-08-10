-- Slow query fixes: companion tick + user brief full-table scans (20260810)

-- DueReminders: WHERE status='pending' AND fire_at <= ? ORDER BY fire_at
-- Existing idx_pet_fire(pet_id, fire_at, status) cannot be used without pet_id.
CREATE INDEX idx_status_fire ON reminders (status, fire_at);

-- GetBrief / Recompile: WHERE pet_id=? AND status='approved' ORDER BY importance DESC, updated_at DESC
CREATE INDEX idx_pet_status_imp ON user_brief_entries (pet_id, status, importance DESC, updated_at DESC);

-- DueTodos: WHERE done=0 AND due_at IS NOT NULL AND due_at <= ? ORDER BY due_at
CREATE INDEX idx_done_due ON todos (done, due_at);
