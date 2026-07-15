ALTER TABLE checkin_configs
    ADD COLUMN IF NOT EXISTS daily_user_reward_cap numeric(20, 6) NOT NULL DEFAULT 10
    CHECK (daily_user_reward_cap > 0);

ALTER TABLE checkin_records
    ADD COLUMN IF NOT EXISTS email text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_checkin_records_workspace_date
    ON checkin_records (user_id, admin_account_id, checkin_date DESC);

CREATE INDEX IF NOT EXISTS idx_checkin_records_workspace_email
    ON checkin_records (user_id, admin_account_id, lower(email));
