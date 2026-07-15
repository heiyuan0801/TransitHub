CREATE TABLE IF NOT EXISTS checkin_configs (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    embed_token text NOT NULL,
    sub2api_source_origin text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    daily_min numeric(20, 6) NOT NULL DEFAULT 0.1 CHECK (daily_min > 0),
    daily_max numeric(20, 6) NOT NULL DEFAULT 1 CHECK (daily_max >= daily_min),
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, admin_account_id)
);

CREATE TABLE IF NOT EXISTS checkin_milestones (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    days integer NOT NULL CHECK (days >= 2 AND days <= 3650),
    bonus_amount numeric(20, 6) NOT NULL CHECK (bonus_amount > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, days),
    FOREIGN KEY (user_id, admin_account_id) REFERENCES checkin_configs(user_id, admin_account_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS checkin_records (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    sub2api_user_id text NOT NULL,
    masked_email text NOT NULL DEFAULT '',
    checkin_date date NOT NULL,
    streak_days integer NOT NULL CHECK (streak_days >= 1),
    base_reward numeric(20, 6) NOT NULL CHECK (base_reward > 0),
    milestone_reward numeric(20, 6) NOT NULL DEFAULT 0 CHECK (milestone_reward >= 0),
    total_reward numeric(20, 6) NOT NULL CHECK (total_reward > 0),
    reward_status text NOT NULL CHECK (reward_status IN ('pending','fulfilled','retryable_failed','failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    idempotency_key text NOT NULL,
    remote_reference text NOT NULL DEFAULT '',
    error_key text NOT NULL DEFAULT '',
    error_detail text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    fulfilled_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_checkin_configs_workspace ON checkin_configs (user_id, admin_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_checkin_configs_token ON checkin_configs (embed_token);
CREATE UNIQUE INDEX IF NOT EXISTS idx_checkin_records_daily ON checkin_records (user_id, admin_account_id, sub2api_user_id, checkin_date);
CREATE UNIQUE INDEX IF NOT EXISTS idx_checkin_records_idempotency ON checkin_records (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_checkin_records_workspace ON checkin_records (user_id, admin_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_checkin_records_user ON checkin_records (user_id, admin_account_id, sub2api_user_id, checkin_date DESC);
